import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Menu, Divider, Popconfirm, message } from 'antd';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
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

  static contextTypes = {
    intl: PropTypes.any,
  };

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
      title: this.context.intl.formatMessage({ id: 'table.modify' }),
      language: this.context.intl.locale,
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
      title: this.context.intl.formatMessage({ id: 'endpoints.import' }),
      language: this.context.intl.locale,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handleBatchDel(selectedIdents: string[]) {
    BatchDel({
      language: this.context.intl.locale,
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
                <a onClick={() => { this.handleModifyBtnClick(record); }}>
                  <FormattedMessage id="table.modify" />
                </a>
                <Divider type="vertical" />
                <Popconfirm title={<FormattedMessage id="table.delete.sure" />} onConfirm={() => { this.handleDeleteBtnClick(record.ident); }}>
                  <a><FormattedMessage id="table.delete" /></a>
                </Popconfirm>
              </span>
            );
          }}
          renderBatchOper={(selectedIdents) => {
            return [
              <Menu.Item key="batch-import">
                <a onClick={() => { this.handleBatchImport(); }}><FormattedMessage id="endpoints.import" /></a>
              </Menu.Item>,
              <Menu.Item key="batch-delete">
                <a onClick={() => { this.handleBatchDel(selectedIdents); }}><FormattedMessage id="endpoints.delete" /></a>
              </Menu.Item>,
            ];
          }}
        />
      </div>
    );
  }
}
export default CreateIncludeNsTree(index);
