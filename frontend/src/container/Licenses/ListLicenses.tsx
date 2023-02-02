import { Table } from 'antd';
import { ColumnsType } from 'antd/lib/table';
import {
	ResizableHeader,
	ResizeTableWrapper,
} from 'components/ResizeTableWrapper';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { License } from 'types/api/licenses/def';
import { PayloadProps } from 'types/api/licenses/getAll';

function ListLicenses({ licenses }: ListLicensesProps): JSX.Element {
	const { t } = useTranslation(['licenses']);

	const columns: ColumnsType<License> = [
		{
			title: t('column_license_status'),
			dataIndex: 'status',
			key: 'status',
			width: 100,
		},
		{
			title: t('column_license_key'),
			dataIndex: 'key',
			key: 'key',
			width: 80,
		},
		{
			title: t('column_valid_from'),
			dataIndex: 'ValidFrom',
			key: 'valid from',
			width: 80,
		},
		{
			title: t('column_valid_until'),
			dataIndex: 'ValidUntil',
			key: 'valid until',
			width: 80,
		},
	];

	return (
		<ResizeTableWrapper columns={columns}>
			<Table
				rowKey="id"
				dataSource={licenses}
				components={{ header: { cell: ResizableHeader } }}
			/>
		</ResizeTableWrapper>
	);
}

interface ListLicensesProps {
	licenses: PayloadProps;
}

export default ListLicenses;
