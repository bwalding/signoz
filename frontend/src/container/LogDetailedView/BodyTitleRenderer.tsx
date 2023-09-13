import { SettingOutlined } from '@ant-design/icons';
import { Dropdown, Menu } from 'antd';

import { TitleWrapper } from './BodyTitleRenderer.styles';
import { BodyTitleRendererProps } from './LogDetailedView.types';

function BodyTitleRenderer({ title }: BodyTitleRendererProps): JSX.Element {
	const menu = (
		<Menu>
			<Menu.Item key="0">Filter for {title}</Menu.Item>
			<Menu.Item key="1">Filter out ${title}</Menu.Item>
		</Menu>
	);

	return (
		<TitleWrapper>
			{title}
			<Dropdown overlay={menu} trigger={['click']}>
				<SettingOutlined style={{ marginLeft: 8 }} className="hover-reveal" />
			</Dropdown>
		</TitleWrapper>
	);
}

export default BodyTitleRenderer;
