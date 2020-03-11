import React from 'react';
import { Card, Tag } from 'antd';
import _ from 'lodash';

const toptMap: any = {
  '=': '包含',
  '!=': '排除',
};

export default function Filter(props: any) {
  const { data, extra } = props;
  const { tkey, topt, tval } = data;

  return (
    <Card
      className="ant-card-small"
      title={
        <span>
          {tkey}
          <span style={{ paddingLeft: 10 }}>{toptMap[topt]}</span>
        </span>
      }
      extra={extra}
    >
      {
        _.map(tval, o => <Tag key={o} className="ant-tag-fix">{o}</Tag>)
      }
    </Card>
  );
}
