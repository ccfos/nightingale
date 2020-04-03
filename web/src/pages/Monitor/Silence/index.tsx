import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
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

class index extends Component<WrappedComponentProps> {
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

  handleDelConfirm = (id: number) => {
    request(`${api.maskconf}/${id}`, {
      method: 'DELETE',
    }).then(() => {
      message.success(this.props.intl.formatMessage({ id: 'msg.delete.success' }));
      this.fetchData();
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
            <Link to={{ pathname: '/monitor/silence/add', search: `nid=${this.selectedNodeId}` }}>
              <FormattedMessage id="silence.add" />
            </Link>
          </Button>
          <Input.Search
            style={{ width: 200, marginLeft: 8 }}
            placeholder="Search"
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
                title: <FormattedMessage id="silence.metric" />,
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
                title: <FormattedMessage id="silence.bindNode" />,
                dataIndex: 'node_path',
              }, {
                title: <FormattedMessage id="silence.time" />,
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
                title: <FormattedMessage id="silence.cause" />,
                dataIndex: 'cause',
                width: 120,
              }, {
                title: <FormattedMessage id="silence.user" />,
                dataIndex: 'user',
              }, {
                title: <FormattedMessage id="table.operations" />,
                width: 60,
                render: (text, record) => (
                  <span>
                    <Popconfirm title={<FormattedMessage id="table.delete.sure" />} onConfirm={() => { this.handleDelConfirm(record.id); }}>
                      <a><FormattedMessage id="table.delete" /></a>
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

export default CreateIncludeNsTree(injectIntl(index), { visible: true });
