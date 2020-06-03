import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
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

class Collect extends Component<WrappedComponentProps> {
  static contextTypes = {
    getNodes: PropTypes.func,
    getSelectedNode: PropTypes.func,
  };
  selectedNodeId: number | undefined = undefined;
  state = {
    loading: false,
    data: [] as any[],
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
      message.success(this.props.intl.formatMessage({ id: 'msg.delete.success' }));
      this.fetchData();
    });
  }

  handleBatchDelete = () => {
    const { selectedRows } = this.state;
    Modal.confirm({
      title: this.props.intl.formatMessage({ id: 'table.delete.batch' }),
      content: this.props.intl.formatMessage({ id: 'table.delete.there.sure' }),
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
          message.success(this.props.intl.formatMessage({ id: 'msg.delete.success' }));
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
        const reqBody = _.map(selectedRows, (item: any) => {
          const pureItem = _.pickBy(item, (v, k) => {
            return !_.includes(['id', 'creator', 'created', 'last_updator', 'last_updated'], k);
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
          message.success(this.props.intl.formatMessage({ id: 'clone.to.other.node.success' }));
          this.fetchData();
        });
      },
    });
  }

  filterData = () => {
    const { searchValue, collectType } = this.state;
    let { data } = this.state;
    if (searchValue) {
      data = _.filter(data, (item: any) => {
        return item.name.indexOf(searchValue) > -1;
      });
    }
    if (collectType) {
      data = _.filter(data, (item: any) => {
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
              placeholder={this.props.intl.formatMessage({ id: 'collect.common.type' })}
              value={this.state.collectType}
              onChange={(value: string) => {
                this.setState({ collectType: value });
              }}
            >
              {
                _.map(typeMap, (value, key) => {
                  return <Select.Option key={key} value={key}><FormattedMessage id={`collect.${key}`} /></Select.Option>;
                })
              }
            </Select>
            <Input.Search
              style={{ width: 200 }}
              onSearch={this.handleSearchChange}
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
                          <Link to={{ pathname: `/monitor/collect/add/${key}` }}><FormattedMessage id={`collect.${key}`} /></Link>
                        </Menu.Item>
                      );
                    })
                  }
                </Menu>
              }
            >
              <Button style={{ marginRight: 8 }}>
                <FormattedMessage id="table.create" /> <Icon type="down" />
              </Button>
            </Dropdown>
            <Dropdown
              overlay={
                <Menu>
                  <Menu.Item>
                    <Button type="link" disabled={!canBatchOper} onClick={this.handleBatchDelete}>
                      <FormattedMessage id="table.delete" />
                    </Button>
                  </Menu.Item>
                  <Menu.Item>
                    <Button type="link" disabled={!canBatchOper} onClick={this.handleBatchCloneToOtherNid}>
                      <FormattedMessage id="clone.to.other.node" />
                    </Button>
                  </Menu.Item>
                </Menu>
              }
            >
              <Button>
                <FormattedMessage id="table.batch.operations" /> <Icon type="down" />
              </Button>
            </Dropdown>
          </Col>
        </Row>
        <Table
          rowKey={(record: any) => record.id + record.collect_type}
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
              title: <FormattedMessage id="collect.common.name" />,
              dataIndex: 'name',
            }, {
              title: <FormattedMessage id="collect.common.type" />,
              dataIndex: 'collect_type',
              render: (text) => {
                return <FormattedMessage id={`collect.${text}`} />;
              },
            }, {
              title: <FormattedMessage id="collect.common.creator" />,
              dataIndex: 'creator',
            }, {
              title: <FormattedMessage id="collect.common.last_updated" />,
              dataIndex: 'last_updated',
              render: (text) => {
                return moment(text).format('YYYY-MM-DD HH:mm:ss');
              },
            }, {
              title: <FormattedMessage id="table.operations" />,
              render: (_text, record: any) => {
                return (
                  <span>
                    <Link to={{ pathname: `/monitor/collect/modify/${_.lowerCase(record.collect_type)}/${record.id}` }}>
                      <FormattedMessage id="table.modify" />
                    </Link>
                    <Divider type="vertical" />
                    <Link to={{ pathname: `/monitor/collect/clone/${_.lowerCase(record.collect_type)}/${record.id}` }}>
                      <FormattedMessage id="table.clone" />
                    </Link>
                    <Divider type="vertical" />
                    <Popconfirm
                      title={<FormattedMessage id="table.delete.sure" />}
                      onConfirm={() => { this.handleDelete(record); }}
                    >
                      <a><FormattedMessage id="table.delete" /></a>
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

export default CreateIncludeNsTree(injectIntl(Collect), { visible: true });
