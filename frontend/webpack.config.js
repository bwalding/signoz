/* eslint-disable @typescript-eslint/no-var-requires */
// shared config (dev and prod)
const { resolve } = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const portFinderSync = require('portfinder-sync');
const dotenv = require('dotenv');
const webpack = require('webpack');
const TsconfigPathsPlugin = require('tsconfig-paths-webpack-plugin');
const { BundleAnalyzerPlugin } = require('webpack-bundle-analyzer');

dotenv.config();

console.log(resolve(__dirname, './src/'));

const cssLoader = 'css-loader';
const sassLoader = 'sass-loader';
const styleLoader = 'style-loader';

const plugins = [
	new HtmlWebpackPlugin({
		template: 'src/index.html.ejs',
		INTERCOM_APP_ID: process.env.INTERCOM_APP_ID,
	}),
	new webpack.ProvidePlugin({
		process: 'process/browser',
	}),
	new webpack.DefinePlugin({
		'process.env': JSON.stringify({
			NODE_ENV: process.env.NODE_ENV,
			FRONTEND_API_ENDPOINT: process.env.FRONTEND_API_ENDPOINT,
			INTERCOM_APP_ID: process.env.INTERCOM_APP_ID,
		}),
	}),
];

if (process.env.BUNDLE_ANALYSER === 'true') {
	plugins.push(new BundleAnalyzerPlugin({ analyzerMode: 'server' }));
}

/**
 * @type {import('webpack').Configuration}
 */
const config = {
	mode: 'development',
	devtool: 'source-map',
	entry: resolve(__dirname, './src/index.tsx'),
	devServer: {
		historyApiFallback: true,
		open: true,
		hot: true,
		liveReload: true,
		port: portFinderSync.getPort(3301),
		static: {
			directory: resolve(__dirname, 'public'),
			publicPath: '/',
			watch: true,
		},
		allowedHosts: 'all',
	},
	target: 'web',
	output: {
		path: resolve(__dirname, './build'),
		publicPath: '/',
	},
	resolve: {
		extensions: ['.ts', '.tsx', '.js', '.jsx'],
		plugins: [new TsconfigPathsPlugin({})],
		fallback: { 'process/browser': require.resolve('process/browser') },
	},
	module: {
		rules: [
			{
				test: [/\.jsx?$/, /\.tsx?$/],
				use: ['babel-loader'],
				exclude: /node_modules/,
			},
			{
				test: /\.mdx?$/,
				use: [
					// `babel-loader` is optional:
					{ loader: 'babel-loader', options: {} },
					{
						loader: '@mdx-js/loader',
						/** @type {import('@mdx-js/loader').Options} */
						options: {
							/* jsxImportSource: …, otherOptions… */
						},
					},
				],
			},
			{
				test: /\.css$/,
				use: [
					styleLoader,
					{
						loader: cssLoader,
						options: {
							modules: true,
						},
					},
				],
			},
			{
				test: /\.(jpe?g|png|gif|svg)$/i,
				use: [
					'file-loader?hash=sha512&digest=hex&name=img/[chunkhash].[ext]',
					'image-webpack-loader?bypassOnDebug&optipng.optimizationLevel=7&gifsicle.interlaced=false',
				],
			},
			{
				test: /\.(ttf|eot|woff|woff2)$/,
				use: ['file-loader'],
			},
			{
				test: /\.less$/i,
				use: [
					{
						loader: styleLoader,
					},
					{
						loader: cssLoader,
						options: {
							modules: true,
						},
					},
					{
						loader: 'less-loader',
						options: {
							lessOptions: {
								javascriptEnabled: true,
							},
						},
					},
				],
			},
			{
				test: /\.s[ac]ss$/i,
				use: [
					// Creates `style` nodes from JS strings
					styleLoader,
					// Translates CSS into CommonJS
					cssLoader,
					// Compiles Sass to CSS
					sassLoader,
				],
			},
		],
	},
	plugins,
	performance: {
		hints: false,
	},
	optimization: {
		minimize: false,
	},
};

module.exports = config;
