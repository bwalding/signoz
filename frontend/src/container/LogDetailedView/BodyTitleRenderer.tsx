import { SettingOutlined } from '@ant-design/icons';
import { Dropdown, MenuProps } from 'antd';
import { OPERATORS } from 'constants/queryBuilder';
import { useActiveLog } from 'hooks/logs/useActiveLog';

import { TitleWrapper } from './BodyTitleRenderer.styles';
import { DROPDOWN_KEY } from './constant';
import { BodyTitleRendererProps } from './LogDetailedView.types';
import {
	generateFieldKeyForArray,
	getDataTypes,
	removeObjectFromString,
} from './utils';

function BodyTitleRenderer({
	title,
	parentIsArray = false,
	nodeKey,
	value,
}: BodyTitleRendererProps): JSX.Element {
	const { onAddToQuery } = useActiveLog();

	const filterHandler = (isFilterIn: boolean) => (): void => {
		if (parentIsArray) {
			onAddToQuery(
				generateFieldKeyForArray(
					removeObjectFromString(nodeKey),
					getDataTypes(value),
				),
				`${value}`,
				isFilterIn ? OPERATORS.HAS : OPERATORS.NHAS,
				true,
				parentIsArray ? getDataTypes([value]) : getDataTypes(value),
			);
		} else {
			onAddToQuery(
				`body.${removeObjectFromString(nodeKey)}`,
				`${value}`,
				isFilterIn ? OPERATORS['='] : OPERATORS['!='],
				true,
				getDataTypes(value),
			);
		}
	};

	const onClickHandler: MenuProps['onClick'] = (props): void => {
		const mapper = {
			[DROPDOWN_KEY.FILTER_IN]: filterHandler(true),
			[DROPDOWN_KEY.FILTER_OUT]: filterHandler(false),
		};

		const handler = mapper[props.key];

		if (handler) {
			handler();
		}
	};

	const menu: MenuProps = {
		items: [
			{
				key: DROPDOWN_KEY.FILTER_IN,
				label: `Filter for ${value}`,
			},
			{
				key: DROPDOWN_KEY.FILTER_OUT,
				label: `Filter out ${value}`,
			},
		],
		onClick: onClickHandler,
	};

	return (
		<TitleWrapper>
			{title.toString()} {!parentIsArray && `- ${value}`}
			<Dropdown menu={menu} trigger={['click']}>
				<SettingOutlined style={{ marginLeft: 8 }} className="hover-reveal" />
			</Dropdown>
		</TitleWrapper>
	);
}

export default BodyTitleRenderer;
