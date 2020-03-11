import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Link } from 'react-router-dom';
import { Row, Col, Input, Divider, Dropdown, Button, Icon, Menu, Select, Popconfirm, Modal, Table, message } from 'antd';
import _ from 'lodash';
import moment from 'moment';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import { typeMap } from './config';
import BatchCloneToNidModal from './BatchCloneToNidModal';

class Collect extends Component {
  static contextTypes = {
    getNodes: PropTypes.func,
    getSelectedNode: PropTypes.func,
  };
  selectedNodeId: number | undefined = undefined;
  state = {
    loading: false,
    data: [],
    collectType: undefined,
    searchValue: '',
    selectedRowKeys: [],
    selectedRows: [],
  };

  componentWillMount = () => {
    const { getSelectedNode } = this.context;
    this.selectedNodeId = getSelectedNode('id');
    this.fetchData();
  }

  componentWillReceiveProps = () => {
    const { getSelectedNode } = this.context;
    const selectedNodeId = getSelectedNode('id');

    if (this.selectedNodeId !== selectedNodeId) {
      this.setState({ selectedRowKeys: [], selectedRows: [] });
      this.selectedNodeId = selectedNodeId;
      this.fetchData();
    }
  }

  fetchData() {
    if (this.selectedNodeId !== undefined) {
      this.setState({ loading: true });
      request(`${api.collect}/list?nid=${this.selectedNodeId}`).then((data) => {
        this.setState({ data });
      }).finally(() => {
        this.setState({ loading: false });
      });
    }
  }

  handleSearchChange = (val: string) => {
    this.setState({ searchValue: val });
  }

  handleDelete = (record: any) => {
    request(api.collect, {
      method: 'DELETE',
      body: JSON.stringify([{
        type: record.collect_type,
        ids: [record.id],
      }]),
    }).then(() => {
      message.success('删除成功！');
      this.fetchData();
    });
  }

  handleBatchDelete = () => {
    const { selectedRows } = this.state;
    Modal.confirm({
      title: '批量删除',
      content: '确定要删除所选的策略吗？',
      onOk: () => {
        const typeGroup = _.groupBy(selectedRows, 'collect_type');
        const reqBody = _.map(typeGroup, (value, key) => {
          return {
            type: key,
            ids: _.map(value, 'id'),
          };
        });
        request(api.collect, {
          method: 'DELETE',
          body: JSON.stringify(reqBody),
        }).then(() => {
          message.success('批量删除成功！');
          this.fetchData();
        });
      },
    });
  }

  handleBatchCloneToOtherNid = () => {
    const { selectedRows } = this.state;
    const { getNodes } = this.context;
    const treeNodes = getNodes();
    BatchCloneToNidModal({
      treeNodes,
      onOk: (nid: number) => {
        const reqBody = _.map(selectedRows, (item) => {
          const pureItem = _.pickBy(item, (v, k) => {
            return !_.includes(['id', 'creator', 'created', 'last_updator', 'last_updated', 'tags'], k);
          });
          return {
            type: item.collect_type,
            data: {
              ...pureItem,
              nid,
            },
          };
        });
        request(api.collect, {
          method: 'POST',
          body: JSON.stringify(reqBody),
        }).then(() => {
          message.success('批量克隆到节点成功!');
          this.fetchData();
        });
      },
    });
  }

  filterData = () => {
    const { searchValue, collectType } = this.state;
    let { data } = this.state;
    if (searchValue) {
      data = _.filter(data, (item) => {
        return item.name.indexOf(searchValue) > -1;
      });
    }
    if (collectType) {
      data = _.filter(data, (item) => {
        return item.collect_type === collectType;
      });
    }
    return data;
  }

  render() {
    const data = this.filterData();
    const { selectedRows } = this.state;
    const canBatchOper = !_.isEmpty(selectedRows);
    return (
      <div>
        <Row>
          <Col span={12} className="mb10">
            <Select
              allowClear
              style={{ width: 100, marginRight: 8 }}
              className="mr10"
              placeholder="类型"
              value={this.state.collectType}
              onChange={(value: string) => {
                this.setState({ collectType: value });
              }}
            >
              {
                _.map(typeMap, (value, key) => {
                  return <Select.Option key={key} value={key}>{value}</Select.Option>;
                })
              }
            </Select>
            <Input.Search
              style={{ width: 200 }}
              onSearch={this.handleSearchChange}
              placeholder="搜索名称"
            />
          </Col>
          <Col span={12} style={{ textAlign: 'right' }}>
            <Dropdown
              overlay={
                <Menu>
                  {
                    _.map(typeMap, (value, key) => {
                      return (
                        <Menu.Item key={key}>
                          <Link to={{ pathname: `/monitor/collect/add/${key}` }}>{value}</Link>
                        </Menu.Item>
                      );
                    })
                  }
                </Menu>
              }
            >
              <Button style={{ marginRight: 8 }}>
                新增采集 <Icon type="down" />
              </Button>
            </Dropdown>
            <Dropdown
              overlay={
                <Menu>
                  <Menu.Item>
                    <Button type="link" disabled={!canBatchOper} onClick={this.handleBatchDelete}>删除配置</Button>
                  </Menu.Item>
                  <Menu.Item>
                    <Button type="link" disabled={!canBatchOper} onClick={this.handleBatchCloneToOtherNid}>克隆到其他节点</Button>
                  </Menu.Item>
                </Menu>
              }
            >
              <Button>
                批量操作 <Icon type="down" />
              </Button>
            </Dropdown>
          </Col>
        </Row>
        <Table
          rowKey={record => record.id + record.collect_type}
          rowSelection={{
            selectedRowKeys: this.state.selectedRowKeys,
            onChange: (selectedRowKeys, selectedRows) => {
              this.setState({
                selectedRowKeys,
                selectedRows,
              });
            },
          }}
          dataSource={data}
          columns= {[
            {
              title: '名称',
              dataIndex: 'name',
            }, {
              title: '类型',
              dataIndex: 'collect_type',
              render: (text) => {
                return typeMap[text];
              },
            }, {
              title: '创建者',
              dataIndex: 'creator',
            }, {
              title: '修改时间',
              dataIndex: 'last_updated',
              render: (text) => {
                return moment(text).format('YYYY-MM-DD HH:mm:ss');
              },
            }, {
              title: '操作',
              render: (text, record) => {
                return (
                  <span>
                    <Link to={{ pathname: `/monitor/collect/modify/${_.lowerCase(record.collect_type)}/${record.id}` }}>修改</Link>
                    <Divider type="vertical" />
                    <Link to={{ pathname: `/monitor/collect/clone/${_.lowerCase(record.collect_type)}/${record.id}` }}>克隆</Link>
                    <Divider type="vertical" />
                    <Popconfirm
                      title="确认删除这条配置吗?"
                      onConfirm={() => { this.handleDelete(record); }}
                    >
                      <a>删除</a>
                    </Popconfirm>
                  </span>
                );
              },
            },
          ]}
        />
      </div>
    );
  }
}

export default CreateIncludeNsTree(Collect, { visible: true });
