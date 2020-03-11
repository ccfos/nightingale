import React, { Component } from 'react';
import { Row, Col, Button, Icon, message, Popconfirm } from 'antd';
import _ from 'lodash';
import Filter from './Filter';
import filterFormModal from './FilterFormModal';
import './style.less';

interface Props {
  value: any[],
  onChange: (values: any) => void,
  readOnly: boolean,
  tags: any,
}

const commonPropDefaultValue = {
  readOnly: false,
  tags: {},
};

export default class Filters extends Component<Props> {

  static defaultProps = {
    ...commonPropDefaultValue,
  };

  addFilter = () => {
    const { tags, value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    filterFormModal({
      title: '添加 Tag 条件',
      tags,
      onOk: (data: any) => {
        if (!_.find(value, { tkey: data.tkey })) {
          valueClone.push(data);
          onChange(valueClone);
        } else {
          message.warning('该 Tag 已存在，请更改 Tag');
        }
      },
    });
  }

  updateFilter = (val: any) => {
    const { tags, value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    filterFormModal({
      title: '修改 Tag 条件',
      tags,
      data: val,
      onOk: (data: any) => {
        if (!_.find(value, { tkey: data.tkey }) || val.tkey === data.tkey) {
          _.remove(valueClone, o => o.tkey === val.tkey);
          valueClone.push(data);
          onChange(valueClone);
        } else {
          message.warning('该 Tag 已存在，请更改 Tag');
        }
      },
    });
  }

  deleteFilter = (val: any) => {
    const { value, onChange } = this.props;
    const valueClone = _.cloneDeep(value);

    _.remove(valueClone, o => o.tkey === val.tkey);
    onChange(valueClone);
  }

  renderFilters() {
    const { readOnly, value } = this.props;
    const filtersEle = _.map(value, (item, i) => {
      return (
        <Col span={12} key={i} style={{ marginTop: 5 }}>
          <Filter
            data={item}
            extra={
              !readOnly &&
              <span className="strategy-filter-operation">
                <Icon type="edit" onClick={() => this.updateFilter(item)} />
                <Popconfirm title="确定要删除该 Tag 条件吗？" onConfirm={() => this.deleteFilter(item)}>
                  <Icon type="cross" />
                </Popconfirm>
              </span>
            }
          />
        </Col>
      );
    });

    return <Row gutter={10}>{ filtersEle }</Row>;
  }

  render() {
    const { readOnly, value } = this.props;

    if (readOnly) {
      return (
        <div className="strategy-filters">
          {this.renderFilters()}
        </div>
      );
    }
    return (
      <div className="strategy-filters">
        <Button
          type="ghost"
          size="default"
          onClick={this.addFilter}
        >
          添加筛选条件
        </Button>
        {
          value.length ? this.renderFilters() : null
        }
      </div>
    );
  }
}
