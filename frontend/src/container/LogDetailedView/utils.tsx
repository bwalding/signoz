import { DataNode } from 'antd/es/tree';

import BodyTitleRenderer from './BodyTitleRenderer';
import { AnyObject } from './LogDetailedView.types';

export const recursiveParseJSON = (obj: string): Record<string, unknown> => {
	try {
		const value = JSON.parse(obj);
		if (typeof value === 'string') {
			return recursiveParseJSON(value);
		}
		return value;
	} catch (e) {
		return {};
	}
};

export function jsonToDataNodes(
	json: Record<string, unknown>,
	parentKey = '',
): DataNode[] {
	return Object.entries(json).map(([key, value]) => {
		const nodeKey = parentKey ? `${parentKey}.${key}` : key;

		if (typeof value === 'object' && value !== null) {
			return {
				key: nodeKey,
				title: key,
				children: jsonToDataNodes(value as Record<string, unknown>, nodeKey),
			};
		}
		return {
			key: nodeKey,
			title: <BodyTitleRenderer title={key} />,
		};
	});
}

export function flattenObject(obj: AnyObject, prefix = ''): AnyObject {
	return Object.keys(obj).reduce((acc: AnyObject, k: string): AnyObject => {
		const pre = prefix.length ? `${prefix}.` : '';
		if (typeof obj[k] === 'object' && obj[k] !== null && !Array.isArray(obj[k])) {
			Object.assign(acc, flattenObject(obj[k], pre + k));
		} else {
			acc[pre + k] = obj[k];
		}
		return acc;
	}, {});
}
