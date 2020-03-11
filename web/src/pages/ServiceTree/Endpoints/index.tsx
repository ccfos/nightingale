import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Menu } from 'antd';
import _ from 'lodash';
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
      selectedNode,
      selectedIdents,
      onOk: () => {
        this.endpointList.reload();
      },
    });
  }

  handleModifyAliasBtnClick = (record: Endpoint) => {
    EditEndpoint({
      title: '修改别名',
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
          请先选择左侧服务节点
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
                <a onClick={() => { this.handleModifyAliasBtnClick(record); }}>改别名</a>
              </span>
            );
          }}
          renderBatchOper={(selectedIdents) => {
            return [
              <Menu.Item key="batch-bind">
                <a onClick={() => { this.handleHostBindBtnClick(); }}>挂载 endpoint</a>
              </Menu.Item>,
              <Menu.Item key="batch-unbind">
                <a onClick={() => { this.handleHostUnbindBtnClick(selectedIdents); }}>解挂 endpoint</a>
              </Menu.Item>,
            ];
          }}
        />
      </div>
    );
  }
}

export default CreateIncludeNsTree(index, { visible: true });
