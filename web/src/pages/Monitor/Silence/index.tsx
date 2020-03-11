import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Link } from 'react-router-dom';
import { Button, Popconfirm, Input, Table, message } from 'antd';
import moment from 'moment';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import { prefixCls } from '../config';
import './style.less';

const nPrefixCls = `${prefixCls}-silence`;
const timeFormatMap = {
  antd: 'yyyy-MM-dd HH:mm:ss',
  moment: 'YYYY-MM-DD HH:mm:ss',
};

class index extends Component {
  static contextTypes = {
    getSelectedNode: PropTypes.func,
  };

  otherParamsKey = ['dept_id'];
  selectedNodeId: number | undefined = undefined;
  state = {
    data: [],
    loading: false,
    filterValue: {
      search: '',
    },
    delBtnLoading: false,
    selectedRowKeys: [],
    selectedNodeId: undefined,
    selectedNode: {},
  };

  componentDidMount() {
    this.fetchData();
  }

  componentWillMount = () => {
    const { getSelectedNode } = this.context;
    this.selectedNodeId = getSelectedNode('id');
  }

  componentWillReceiveProps = () => {
    const { getSelectedNode } = this.context;
    const selectedNode = getSelectedNode();

    if (!_.isEqual(selectedNode, this.state.selectedNode)) {
      this.setState({
        selectedNode,
      }, () => {
        this.selectedNodeId = getSelectedNode('id');
        this.fetchData();
      });
    }
  }

  fetchData() {
    if (this.selectedNodeId) {
      request(`${api.node}/${this.selectedNodeId}/maskconf`).then((res) => {
        this.setState({ data: res || [] });
      });
    }
  }

  handleBatchDelConfirm = () => {
    const { selectedRowKeys } = this.state;
    request(`${api.maskconf}/${selectedRowKeys}`, {
      method: 'DELETE',
    }).then(() => {
      this.setState({ selectedRowKeys: [] });
      message.success('批量解除成功！');
      this.fetchData();
    }).catch(() => {
      message.error('批量解除失败！');
    });
  }

  handleDelConfirm = (id: number) => {
    request(`${api.maskconf}/${id}`, {
      method: 'DELETE',
    }).then(() => {
      message.success('解除成功！');
      this.fetchData();
    }).catch(() => {
      message.error('解除失败！');
    });
  }

  filterData() {
    const { data, filterValue } = this.state;
    const { search = '' } = filterValue;
    const reg = new RegExp(search);

    return _.filter(data, (item: any) => {
      if (search) {
        const metric = item.metric || '';
        const endpoints = item.endpoints || '';
        const cause = item.cause || '';
        if (!reg.test(metric) && !reg.test(endpoints) && !reg.test(cause)) {
          return false;
        }
      }
      return true;
    });
  }

  render() {
    const { filterValue } = this.state;
    const data = this.filterData();

    return (
      <div className={nPrefixCls}>
        <div className={`${nPrefixCls}-operationbar`} style={{ marginBottom: 10 }}>
          <Button
            style={{ marginRight: 8 }}
          >
            <Link to={{ pathname: '/monitor/silence/add', search: `nid=${this.selectedNodeId}` }}>新增屏蔽</Link>
          </Button>
          <Input.Search
            style={{ width: 200, marginLeft: 8 }}
            placeholder="搜索"
            value={filterValue.search}
            onChange={(e) => {
              this.setState({
                filterValue: {
                  ...filterValue,
                  search: e.target.value,
                },
              });
            }}
          />
        </div>
        <div className="alarm-strategy-content">
          <Table
            rowKey="id"
            dataSource={data}
            columns={[
              {
                title: '指标',
                dataIndex: 'metric',
                width: 150,
                render: (text, record) => {
                  return (
                    <div>
                      <div>{text}</div>
                      <div>{record.tags}</div>
                    </div>
                  );
                },
              }, {
                title: 'Endpoints',
                dataIndex: 'endpoints',
                render(text) {
                  return _.map(text, (item) => {
                    return <div key={item}>{item}</div>;
                  });
                },
              }, {
                title: '关联节点',
                dataIndex: 'node_path',
              }, {
                title: '屏蔽时间',
                width: 180,
                render(text, record) {
                  const beginTs = record.btime;
                  const endTs = record.etime;
                  if (beginTs && endTs) {
                    return (
                      <span>
                        {moment(beginTs * 1000).format(timeFormatMap.moment)} ~ {moment(endTs * 1000).format(timeFormatMap.moment)}
                      </span>
                    );
                  }
                  return <span>unknown</span>;
                },
              }, {
                title: '屏蔽原因',
                dataIndex: 'cause',
                width: 120,
              }, {
                title: '操作者',
                dataIndex: 'user',
              }, {
                title: '操作',
                width: 60,
                render: (text, record) => (
                  <span>
                    <Popconfirm title="确定要解除这个策略吗？" onConfirm={() => { this.handleDelConfirm(record.id); }}>
                      <a>解除</a>
                    </Popconfirm>
                  </span>
                ),
              },
            ]}
          />
        </div>
      </div>
    );
  }
}

export default CreateIncludeNsTree(index, { visible: true });
