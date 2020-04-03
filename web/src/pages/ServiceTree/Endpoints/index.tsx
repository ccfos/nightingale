import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Menu } from 'antd';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import exportXlsx from '@common/exportXlsx';
import api from '@common/api';
import { Endpoint } from '@interface';
import EndpointList from '@cpts/EndpointList';
import EditEndpoint from '@cpts/EndpointList/Edit';
import BatchBind from './BatchBind';
import BatchUnbind from './BatchUnbind';

class index extends Component {
  endpointList: any;
  selectedNodeId: number | undefined = undefined;
  static contextTypes = {
    getSelectedNode: PropTypes.func,
    intl: PropTypes.any,
  };

  componentWillMount = () => {
    const { getSelectedNode } = this.context;
    this.selectedNodeId = getSelectedNode('id');
  }

  componentWillReceiveProps = () => {
    const { getSelectedNode } = this.context;
    const selectedNodeId = getSelectedNode('id');

    if (this.selectedNodeId !== selectedNodeId) {
      this.selectedNodeId = selectedNodeId;
      if (this.endpointList) {
        this.endpointList.setState({
          selectedRowKeys: [],
          selectedIps: [],
          selectedHosts: [],
        });
      }
    }
  }

  async exportEndpoints(endpoints: Endpoint[]) {
    const data = _.map(endpoints, (item) => {
      return {
        ...item,
        nodes: _.join(item.nodes),
      };
    });
    exportXlsx(data);
  }

  handleHostBindBtnClick = () => {
    const { getSelectedNode } = this.context;
    const selectedNode = getSelectedNode();
    BatchBind({
      title: this.context.intl.formatMessage({ id: 'endpoints.bind' }),
      language: this.context.intl.locale,
      selectedNode,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handleHostUnbindBtnClick = (selectedIdents: string[]) => {
    const { getSelectedNode } = this.context;
    const selectedNode = getSelectedNode();
    BatchUnbind({
      title: this.context.intl.formatMessage({ id: 'endpoints.unbind' }),
      language: this.context.intl.locale,
      selectedNode,
      selectedIdents,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handleModifyAliasBtnClick = (record: Endpoint) => {
    EditEndpoint({
      title: this.context.intl.formatMessage({ id: 'table.modify' }),
      language: this.context.intl.locale,
      data: record,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  render() {
    if (!this.selectedNodeId) {
      return (
        <div>
          <FormattedMessage id="please.select.node" />
        </div>
      );
    }
    return (
      <div>
        <EndpointList
          ref={(ref) => { this.endpointList = ref; }}
          fetchUrl={`${api.node}/${this.selectedNodeId}/endpoint`}
          columnKeys={['ident', 'alias', 'nodes']}
          exportEndpoints={this.exportEndpoints}
          renderOper={(record) => {
            return (
              <span>
                <a onClick={() => { this.handleModifyAliasBtnClick(record); }}>
                  <FormattedMessage id="endpoints.modify.alias" />
                </a>
              </span>
            );
          }}
          renderBatchOper={(selectedIdents) => {
            return [
              <Menu.Item key="batch-bind">
                <a onClick={() => { this.handleHostBindBtnClick(); }}>
                  <FormattedMessage id="endpoints.bind" />
                </a>
              </Menu.Item>,
              <Menu.Item key="batch-unbind">
                <a onClick={() => { this.handleHostUnbindBtnClick(selectedIdents); }}>
                  <FormattedMessage id="endpoints.unbind" />
                </a>
              </Menu.Item>,
            ];
          }}
        />
      </div>
    );
  }
}

export default CreateIncludeNsTree(index, { visible: true });
