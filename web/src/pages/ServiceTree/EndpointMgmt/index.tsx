import React, { Component } from 'react';
import { Menu, Divider, Popconfirm, message } from 'antd';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import exportXlsx from '@common/exportXlsx';
import request from '@common/request';
import api from '@common/api';
import EndpointList from '@cpts/EndpointList';
import EditEndpoint from '@cpts/EndpointList/Edit';
import { Endpoint } from '@interface';
import BatchDel from './BatchDel';
import BatchImport from './BatchImport';

class index extends Component {
  endpointList: any;
  state = {};

  async exportEndpoints(endpoints: Endpoint[]) {
    const data = _.map(endpoints, (item) => {
      return {
        ...item,
        nodes: _.join(item.nodes),
      };
    });
    exportXlsx(data);
  }

  handleModifyBtnClick(record: Endpoint) {
    EditEndpoint({
      title: '修改信息',
      type: 'admin',
      data: record,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handleDeleteBtnClick(ident: string) {
    request(api.endpoint, {
      method: 'DELETE',
      body: JSON.stringify({
        idents: [ident],
      }),
    }).then(() => {
      this.endpointList.reload();
      message.success('删除成功！');
    });
  }

  handleBatchImport() {
    BatchImport({
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handleBatchDel(selectedIdents: string[]) {
    BatchDel({
      selectedIdents,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handlePaginationChange = () => {
    this.setState({ selectedRowKeys: [], selectedIps: [], selectedHosts: [] });
  }

  render() {
    return (
      <div>
        <EndpointList
          ref={(ref) => { this.endpointList = ref; }}
          type="mgmt"
          fetchUrl={api.endpoint}
          columnKeys={['ident', 'alias', 'nodes']}
          exportEndpoints={this.exportEndpoints}
          renderOper={(record) => {
            return (
              <span>
                <a onClick={() => { this.handleModifyBtnClick(record); }}>修改</a>
                <Divider type="vertical" />
                <Popconfirm title="确认要删除吗？" onConfirm={() => { this.handleDeleteBtnClick(record.ident); }}>
                  <a>删除</a>
                </Popconfirm>
              </span>
            );
          }}
          renderBatchOper={(selectedIdents) => {
            return [
              <Menu.Item key="batch-import">
                <a onClick={() => { this.handleBatchImport(); }}>导入 endpoints</a>
              </Menu.Item>,
              <Menu.Item key="batch-delete">
                <a onClick={() => { this.handleBatchDel(selectedIdents); }}>删除 endpoints</a>
              </Menu.Item>,
            ];
          }}
        />
      </div>
    );
  }
}
export default CreateIncludeNsTree(index);
