package querier

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	logsV3 "go.signoz.io/signoz/pkg/query-service/app/logs/v3"
	metricsV3 "go.signoz.io/signoz/pkg/query-service/app/metrics/v3"
	"go.signoz.io/signoz/pkg/query-service/app/queryBuilder"
	tracesV3 "go.signoz.io/signoz/pkg/query-service/app/traces/v3"

	"go.signoz.io/signoz/pkg/query-service/cache"
	"go.signoz.io/signoz/pkg/query-service/cache/status"
	"go.signoz.io/signoz/pkg/query-service/interfaces"
	"go.signoz.io/signoz/pkg/query-service/model"
	v3 "go.signoz.io/signoz/pkg/query-service/model/v3"
	"go.uber.org/zap"
)

type channelResult struct {
	Series []*v3.Series
	Err    error
	Name   string
	Query  string
}

type missInterval struct {
	start, end int64 // in milliseconds
}

type querier struct {
	cache        cache.Cache
	reader       interfaces.Reader
	keyGenerator cache.KeyGenerator

	fluxInterval time.Duration

	builder *queryBuilder.QueryBuilder

	// used for testing
	// TODO(srikanthccv): remove this once we have a proper mock
	testingMode     bool
	queriesExecuted []string
	returnedSeries  []*v3.Series
	returnedErr     error
}

type QuerierOptions struct {
	Reader       interfaces.Reader
	Cache        cache.Cache
	KeyGenerator cache.KeyGenerator
	FluxInterval time.Duration

	// used for testing
	TestingMode    bool
	ReturnedSeries []*v3.Series
	ReturnedErr    error
}

func NewQuerier(opts QuerierOptions) interfaces.Querier {
	return &querier{
		cache:        opts.Cache,
		reader:       opts.Reader,
		keyGenerator: opts.KeyGenerator,
		fluxInterval: opts.FluxInterval,

		builder: queryBuilder.NewQueryBuilder(queryBuilder.QueryBuilderOptions{
			BuildTraceQuery:  tracesV3.PrepareTracesQuery,
			BuildLogQuery:    logsV3.PrepareLogsQuery,
			BuildMetricQuery: metricsV3.PrepareMetricQuery,
		}),

		testingMode:    opts.TestingMode,
		returnedSeries: opts.ReturnedSeries,
		returnedErr:    opts.ReturnedErr,
	}
}

func (q *querier) execClickHouseQuery(ctx context.Context, query string) ([]*v3.Series, error) {
	q.queriesExecuted = append(q.queriesExecuted, query)
	if q.testingMode && q.reader == nil {
		return q.returnedSeries, q.returnedErr
	}
	return q.reader.GetTimeSeriesResultV3(ctx, query)
}

func (q *querier) execPromQuery(ctx context.Context, params *model.QueryRangeParams) ([]*v3.Series, error) {
	q.queriesExecuted = append(q.queriesExecuted, params.Query)
	if q.testingMode && q.reader == nil {
		return q.returnedSeries, q.returnedErr
	}
	promResult, _, err := q.reader.GetQueryRangeResult(ctx, params)
	if err != nil {
		return nil, err
	}
	matrix, promErr := promResult.Matrix()
	if promErr != nil {
		return nil, promErr
	}
	var seriesList []*v3.Series
	for _, v := range matrix {
		var s v3.Series
		s.Labels = v.Metric.Copy().Map()
		for idx := range v.Points {
			p := v.Points[idx]
			s.Points = append(s.Points, v3.Point{Timestamp: p.T, Value: p.V})
		}
		seriesList = append(seriesList, &s)
	}
	return seriesList, nil
}

// findMissingTimeRanges finds the missing time ranges in the seriesList
// and returns a list of miss structs, It takes the fluxInterval into
// account to find the missing time ranges.
//
// The [End - fluxInterval, End] is always added to the list of misses, because
// the data might still be in flux and not yet available in the database.
func findMissingTimeRanges(start, end int64, seriesList []*v3.Series, fluxInterval time.Duration) (misses []missInterval) {
	var cachedStart, cachedEnd int64
	for idx := range seriesList {
		series := seriesList[idx]
		for pointIdx := range series.Points {
			point := series.Points[pointIdx]
			if cachedStart == 0 || point.Timestamp < cachedStart {
				cachedStart = point.Timestamp
			}
			if cachedEnd == 0 || point.Timestamp > cachedEnd {
				cachedEnd = point.Timestamp
			}
		}
	}

	// Exclude the flux interval from the cached end time
	cachedEnd = int64(
		math.Min(
			float64(cachedEnd),
			float64(time.Now().UnixMilli()-fluxInterval.Milliseconds()),
		),
	)

	// There are five cases to consider
	// 1. Cached time range is a subset of the requested time range
	// 2. Cached time range is a superset of the requested time range
	// 3. Cached time range is a left overlap of the requested time range
	// 4. Cached time range is a right overlap of the requested time range
	// 5. Cached time range is a disjoint of the requested time range
	if cachedStart >= start && cachedEnd <= end {
		// Case 1: Cached time range is a subset of the requested time range
		// Add misses for the left and right sides of the cached time range
		misses = append(misses, missInterval{start: start, end: cachedStart - 1})
		misses = append(misses, missInterval{start: cachedEnd + 1, end: end})
	} else if cachedStart <= start && cachedEnd >= end {
		// Case 2: Cached time range is a superset of the requested time range
		// No misses
	} else if cachedStart <= start && cachedEnd >= start {
		// Case 3: Cached time range is a left overlap of the requested time range
		// Add a miss for the left side of the cached time range
		misses = append(misses, missInterval{start: cachedEnd + 1, end: end})
	} else if cachedStart <= end && cachedEnd >= end {
		// Case 4: Cached time range is a right overlap of the requested time range
		// Add a miss for the right side of the cached time range
		misses = append(misses, missInterval{start: start, end: cachedStart - 1})
	} else {
		// Case 5: Cached time range is a disjoint of the requested time range
		// Add a miss for the entire requested time range
		misses = append(misses, missInterval{start: start, end: end})
	}

	// remove the struts with start > end
	var validMisses []missInterval
	for idx := range misses {
		miss := misses[idx]
		if miss.start <= miss.end {
			validMisses = append(validMisses, miss)
		}
	}
	return validMisses
}

// findMissingTimeRanges finds the missing time ranges in the cached data
// and returns them as a list of misses
func (q *querier) findMissingTimeRanges(start, end, step int64, cachedData []byte) (misses []missInterval) {
	var cachedSeriesList []*v3.Series
	if err := json.Unmarshal(cachedData, &cachedSeriesList); err != nil {
		// In case of error, we return the entire range as a miss
		return []missInterval{{start: start, end: end}}
	}
	return findMissingTimeRanges(start, end, cachedSeriesList, q.fluxInterval)
}

func labelsToString(labels map[string]string) string {
	type label struct {
		Key   string
		Value string
	}
	var labelsList []label
	for k, v := range labels {
		labelsList = append(labelsList, label{Key: k, Value: v})
	}
	sort.Slice(labelsList, func(i, j int) bool {
		return labelsList[i].Key < labelsList[j].Key
	})
	labelKVs := make([]string, len(labelsList))
	for idx := range labelsList {
		labelKVs[idx] = labelsList[idx].Key + "=" + labelsList[idx].Value
	}
	return fmt.Sprintf("{%s}", strings.Join(labelKVs, ","))
}

func mergeSerieses(cachedSeries, missedSeries []*v3.Series) []*v3.Series {
	// Merge the missed series with the cached series by timestamp
	mergedSeries := make([]*v3.Series, 0)
	seriesesByLabels := make(map[string]*v3.Series)
	for idx := range cachedSeries {
		series := cachedSeries[idx]
		seriesesByLabels[labelsToString(series.Labels)] = series
	}

	for idx := range missedSeries {
		series := missedSeries[idx]
		if _, ok := seriesesByLabels[labelsToString(series.Labels)]; !ok {
			seriesesByLabels[labelsToString(series.Labels)] = series
			continue
		}
		seriesesByLabels[labelsToString(series.Labels)].Points = append(seriesesByLabels[labelsToString(series.Labels)].Points, series.Points...)
	}
	// Sort the points in each series by timestamp
	for idx := range seriesesByLabels {
		series := seriesesByLabels[idx]
		series.SortPoints()
		mergedSeries = append(mergedSeries, series)
	}
	return mergedSeries
}

func (q *querier) runBuilderQueries(ctx context.Context, params *v3.QueryRangeParamsV3, keys map[string]v3.AttributeKey) ([]*v3.Result, error, map[string]string) {

	cacheKeys := q.keyGenerator.GenerateKeys(params)

	ch := make(chan channelResult, len(params.CompositeQuery.BuilderQueries))
	var wg sync.WaitGroup

	for queryName, builderQuery := range params.CompositeQuery.BuilderQueries {
		if builderQuery.Disabled {
			continue
		}
		wg.Add(1)
		if queryName == builderQuery.Expression {
			go q.runBuilderQuery(ctx, builderQuery, params, keys, cacheKeys, ch, &wg)
		} else {
			go q.runBuilderExpression(ctx, builderQuery, params, keys, cacheKeys, ch, &wg)
		}
	}

	wg.Wait()
	close(ch)

	results := make([]*v3.Result, 0)
	errQueriesByName := make(map[string]string)
	var errs []error

	for result := range ch {
		if result.Err != nil {
			errs = append(errs, result.Err)
			errQueriesByName[result.Name] = result.Err.Error()
			continue
		}
		results = append(results, &v3.Result{
			QueryName: result.Name,
			Series:    result.Series,
		})
	}

	var err error
	if len(errs) > 0 {
		err = fmt.Errorf("error in builder queries")
	}

	return results, err, errQueriesByName
}

func (q *querier) runPromQueries(ctx context.Context, params *v3.QueryRangeParamsV3) ([]*v3.Result, error, map[string]string) {
	channelResults := make(chan channelResult, len(params.CompositeQuery.ClickHouseQueries))
	var wg sync.WaitGroup
	cacheKeys := q.keyGenerator.GenerateKeys(params)

	for queryName, promQuery := range params.CompositeQuery.PromQueries {
		wg.Add(1)
		go func(queryName string, promQuery *v3.PromQuery) {
			defer wg.Done()
			cacheKey := cacheKeys[queryName]
			var cachedData []byte
			// Ensure NoCache is not set and cache is not nil
			if !params.NoCache && q.cache != nil {
				data, retrieveStatus, err := q.cache.Retrieve(cacheKey, true)
				zap.L().Debug("cache retrieve status", zap.String("status", retrieveStatus.String()))
				if err == nil {
					cachedData = data
				}
			}
			misses := q.findMissingTimeRanges(params.Start, params.End, params.Step, cachedData)
			missedSeries := make([]*v3.Series, 0)
			cachedSeries := make([]*v3.Series, 0)
			for _, miss := range misses {
				query := metricsV3.BuildPromQuery(promQuery, params.Step, miss.start, miss.end)
				series, err := q.execPromQuery(ctx, query)
				if err != nil {
					channelResults <- channelResult{Err: err, Name: queryName, Query: query.Query, Series: nil}
					return
				}
				missedSeries = append(missedSeries, series...)
			}
			if err := json.Unmarshal(cachedData, &cachedSeries); err != nil && cachedData != nil {
				// ideally we should not be getting an error here
				zap.L().Error("error unmarshalling cached data", zap.Error(err))
			}
			mergedSeries := mergeSerieses(cachedSeries, missedSeries)

			channelResults <- channelResult{Err: nil, Name: queryName, Query: promQuery.Query, Series: mergedSeries}

			// Cache the seriesList for future queries
			if len(missedSeries) > 0 && !params.NoCache && q.cache != nil {
				mergedSeriesData, err := json.Marshal(mergedSeries)
				if err != nil {
					zap.L().Error("error marshalling merged series", zap.Error(err))
					return
				}
				err = q.cache.Store(cacheKey, mergedSeriesData, time.Hour)
				if err != nil {
					zap.L().Error("error storing merged series", zap.Error(err))
					return
				}
			}
		}(queryName, promQuery)
	}
	wg.Wait()
	close(channelResults)

	results := make([]*v3.Result, 0)
	errQueriesByName := make(map[string]string)
	var errs []error

	for result := range channelResults {
		if result.Err != nil {
			errs = append(errs, result.Err)
			errQueriesByName[result.Name] = result.Err.Error()
			continue
		}
		results = append(results, &v3.Result{
			QueryName: result.Name,
			Series:    result.Series,
		})
	}

	var err error
	if len(errs) > 0 {
		err = fmt.Errorf("error in prom queries")
	}

	return results, err, errQueriesByName
}

func (q *querier) runClickHouseQueries(ctx context.Context, params *v3.QueryRangeParamsV3) ([]*v3.Result, error, map[string]string) {
	channelResults := make(chan channelResult, len(params.CompositeQuery.ClickHouseQueries))
	var wg sync.WaitGroup
	for queryName, clickHouseQuery := range params.CompositeQuery.ClickHouseQueries {
		wg.Add(1)
		go func(queryName string, clickHouseQuery *v3.ClickHouseQuery) {
			defer wg.Done()
			series, err := q.execClickHouseQuery(ctx, clickHouseQuery.Query)
			channelResults <- channelResult{Err: err, Name: queryName, Query: clickHouseQuery.Query, Series: series}
		}(queryName, clickHouseQuery)
	}
	wg.Wait()
	close(channelResults)

	results := make([]*v3.Result, 0)
	errQueriesByName := make(map[string]string)
	var errs []error

	for result := range channelResults {
		if result.Err != nil {
			errs = append(errs, result.Err)
			errQueriesByName[result.Name] = result.Err.Error()
			continue
		}
		results = append(results, &v3.Result{
			QueryName: result.Name,
			Series:    result.Series,
		})
	}

	var err error
	if len(errs) > 0 {
		err = fmt.Errorf("error in clickhouse queries")
	}
	return results, err, errQueriesByName
}

func (q *querier) QueryRange(ctx context.Context, params *v3.QueryRangeParamsV3, keys map[string]v3.AttributeKey) ([]*v3.Result, error, map[string]string) {
	var results []*v3.Result
	var err error
	var errQueriesByName map[string]string
	if params.CompositeQuery != nil {
		switch params.CompositeQuery.QueryType {
		case v3.QueryTypeBuilder:
			results, err, errQueriesByName = q.runBuilderQueries(ctx, params, keys)
		case v3.QueryTypePromQL:
			results, err, errQueriesByName = q.runPromQueries(ctx, params)
		case v3.QueryTypeClickHouseSQL:
			results, err, errQueriesByName = q.runClickHouseQueries(ctx, params)
		default:
			err = fmt.Errorf("invalid query type")
		}
	}
	return results, err, errQueriesByName
}

func (q *querier) QueriesExecuted() []string {
	return q.queriesExecuted
}

func (q *querier) runBuilderQuery(
	ctx context.Context,
	builderQuery *v3.BuilderQuery,
	params *v3.QueryRangeParamsV3,
	keys map[string]v3.AttributeKey,
	cacheKeys map[string]string,
	ch chan channelResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	queryName := builderQuery.QueryName

	// TODO: handle other data sources
	if builderQuery.DataSource == v3.DataSourceLogs {
		query, err := logsV3.PrepareLogsQuery(params.Start, params.End, params.CompositeQuery.QueryType, params.CompositeQuery.PanelType, builderQuery)
		if err != nil {
			ch <- channelResult{Err: err, Name: queryName, Query: query, Series: nil}
			return
		}
		series, err := q.execClickHouseQuery(ctx, query)
		ch <- channelResult{Err: err, Name: queryName, Query: query, Series: series}
		return
	}

	if builderQuery.DataSource == v3.DataSourceTraces {
		query, err := tracesV3.PrepareTracesQuery(params.Start, params.End, params.CompositeQuery.QueryType, params.CompositeQuery.PanelType, builderQuery, keys)
		if err != nil {
			ch <- channelResult{Err: err, Name: queryName, Query: query, Series: nil}
			return
		}

		series, err := q.execClickHouseQuery(ctx, query)
		ch <- channelResult{Err: err, Name: queryName, Query: query, Series: series}
		return
	}

	cacheKey := cacheKeys[queryName]
	var cachedData []byte
	if !params.NoCache && q.cache != nil {
		var retrieveStatus status.RetrieveStatus
		data, retrieveStatus, err := q.cache.Retrieve(cacheKey, true)
		zap.L().Debug("cache retrieve status", zap.String("status", retrieveStatus.String()))
		if err == nil {
			cachedData = data
		}
	}
	misses := q.findMissingTimeRanges(params.Start, params.End, params.Step, cachedData)
	missedSeries := make([]*v3.Series, 0)
	cachedSeries := make([]*v3.Series, 0)
	for _, miss := range misses {
		query, err := metricsV3.PrepareMetricQuery(
			miss.start,
			miss.end,
			params.CompositeQuery.QueryType,
			params.CompositeQuery.PanelType,
			builderQuery,
		)
		if err != nil {
			ch <- channelResult{
				Err:    err,
				Name:   queryName,
				Query:  query,
				Series: nil,
			}
			return
		}
		series, err := q.execClickHouseQuery(ctx, query)
		if err != nil {
			ch <- channelResult{
				Err:    err,
				Name:   queryName,
				Query:  query,
				Series: nil,
			}
			return
		}
		missedSeries = append(missedSeries, series...)
	}
	if err := json.Unmarshal(cachedData, &cachedSeries); err != nil && cachedData != nil {
		zap.L().Error("error unmarshalling cached data", zap.Error(err))
	}
	mergedSeries := mergeSerieses(cachedSeries, missedSeries)

	ch <- channelResult{
		Err:    nil,
		Name:   queryName,
		Series: mergedSeries,
	}
	// Cache the seriesList for future queries
	if len(missedSeries) > 0 && !params.NoCache && q.cache != nil {
		mergedSeriesData, err := json.Marshal(mergedSeries)
		if err != nil {
			zap.L().Error("error marshalling merged series", zap.Error(err))
			return
		}
		err = q.cache.Store(cacheKey, mergedSeriesData, time.Hour)
		if err != nil {
			zap.L().Error("error storing merged series", zap.Error(err))
			return
		}
	}
}

func (q *querier) runBuilderExpression(
	ctx context.Context,
	builderQuery *v3.BuilderQuery,
	params *v3.QueryRangeParamsV3,
	keys map[string]v3.AttributeKey,
	cacheKeys map[string]string,
	ch chan channelResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	queryName := builderQuery.QueryName

	queries, err := q.builder.PrepareQueries(params, keys)
	fmt.Println("queries", queries)
	if err != nil {
		ch <- channelResult{Err: err, Name: queryName, Query: "", Series: nil}
		return
	}

	if _, ok := cacheKeys[queryName]; !ok {
		query := queries[queryName]
		series, err := q.execClickHouseQuery(ctx, query)
		ch <- channelResult{Err: err, Name: queryName, Query: query, Series: series}
		return
	}

	cacheKey := cacheKeys[queryName]
	var cachedData []byte
	if !params.NoCache && q.cache != nil {
		var retrieveStatus status.RetrieveStatus
		data, retrieveStatus, err := q.cache.Retrieve(cacheKey, true)
		zap.L().Debug("cache retrieve status", zap.String("status", retrieveStatus.String()))
		if err == nil {
			cachedData = data
		}
	}
	misses := q.findMissingTimeRanges(params.Start, params.End, params.Step, cachedData)
	missedSeries := make([]*v3.Series, 0)
	cachedSeries := make([]*v3.Series, 0)
	for _, miss := range misses {
		missQueries, _ := q.builder.PrepareQueries(&v3.QueryRangeParamsV3{
			Start:          miss.start,
			End:            miss.end,
			Step:           params.Step,
			NoCache:        params.NoCache,
			CompositeQuery: params.CompositeQuery,
			Variables:      params.Variables,
		}, keys)
		query := missQueries[queryName]
		series, err := q.execClickHouseQuery(ctx, query)
		if err != nil {
			ch <- channelResult{Err: err, Name: queryName, Query: query, Series: nil}
			return
		}
		missedSeries = append(missedSeries, series...)
	}
	if err := json.Unmarshal(cachedData, &cachedSeries); err != nil && cachedData != nil {
		zap.L().Error("error unmarshalling cached data", zap.Error(err))
	}
	mergedSeries := mergeSerieses(cachedSeries, missedSeries)

	ch <- channelResult{
		Err:    nil,
		Name:   queryName,
		Series: mergedSeries,
	}
	// Cache the seriesList for future queries
	if len(missedSeries) > 0 && !params.NoCache && q.cache != nil {
		mergedSeriesData, err := json.Marshal(mergedSeries)
		if err != nil {
			zap.L().Error("error marshalling merged series", zap.Error(err))
			return
		}
		err = q.cache.Store(cacheKey, mergedSeriesData, time.Hour)
		if err != nil {
			zap.L().Error("error storing merged series", zap.Error(err))
			return
		}
	}
}
