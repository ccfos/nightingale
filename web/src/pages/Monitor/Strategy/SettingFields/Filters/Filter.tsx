import React from 'react';
import { Card, Tag } from 'antd';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';

const toptMap = {
  '=': 'stra.tag.include',
  '!=': 'stra.tag.exclude',
};

export default function Filter(props) {
  const { data, extra } = props;
  const { tkey, topt, tval } = data;

  return (
    <Card
      className="ant-card-small"
      title={
        <span>
          {tkey}
          <span style={{ paddingLeft: 10 }}>{<FormattedMessage id={toptMap[topt]} />}</span>
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
