import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button } from 'antd';
import {
	initialQueryBuilderFormValuesMap,
	PANEL_TYPES,
} from 'constants/queryBuilder';
import { queryParamNamesMap } from 'constants/queryBuilderQueryNames';
import ROUTES from 'constants/routes';
import { useGetCompositeQueryParam } from 'hooks/queryBuilder/useGetCompositeQueryParam';
import { useQueryBuilder } from 'hooks/queryBuilder/useQueryBuilder';
import { useCallback } from 'react';
import { useHistory } from 'react-router-dom';
import { DataSource } from 'types/common/queryBuilder';

import { constructCompositeQuery } from '../constants';

function BackButton(): JSX.Element {
	const history = useHistory();

	const { updateAllQueriesOperators, resetQuery } = useQueryBuilder();

	const compositeQuery = useGetCompositeQueryParam();

	const handleBack = useCallback(() => {
		if (!compositeQuery) return;

		const nextCompositeQuery = constructCompositeQuery({
			query: compositeQuery,
			initialQueryData: initialQueryBuilderFormValuesMap.logs,
			customQueryData: { disabled: false },
		});

		const updatedQuery = updateAllQueriesOperators(
			nextCompositeQuery,
			PANEL_TYPES.LIST,
			DataSource.LOGS,
		);

		resetQuery(updatedQuery);

		const JSONCompositeQuery = encodeURIComponent(JSON.stringify(updatedQuery));

		const path = `${ROUTES.LOGS_EXPLORER}?${queryParamNamesMap.compositeQuery}=${JSONCompositeQuery}`;

		history.push(path);
	}, [history, compositeQuery, resetQuery, updateAllQueriesOperators]);

	return (
		<Button icon={<ArrowLeftOutlined />} onClick={handleBack}>
			Exit live view
		</Button>
	);
}

export default BackButton;
