import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Row, Col, Input, Button, Checkbox, Popconfirm, message } from 'antd';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import * as config from '@common/config';
import { TreeNode } from '@interface';

interface State {
  selectedNode: TreeNode | undefined,
  selectedNodeName: string | undefined,
  newNodeName: string | undefined,
  newNodeLeaf: boolean,
}

function updatePathByName(path: string, name: string) {
  if (_.includes(path, '.')) {
    const pathArr = _.split(path, '.');
    pathArr[pathArr.length - 1] = name;
    return _.join(pathArr, '.');
  }
  return path;
}

class index extends Component<null, State> {
  static contextTypes = {
    getSelectedNode: PropTypes.func,
    updateSelectedNode: PropTypes.func,
    deleteSelectedNode: PropTypes.func,
    reloadNsTree: PropTypes.func,
  };

  state = {
    selectedNode: undefined,
    selectedNodeName: undefined,
    newNodeName: undefined,
    newNodeLeaf: false,
  } as State;

  componentWillMount = () => {
    const selectedNode = this.getSelectedNode();
    this.setState({
      selectedNode,
      selectedNodeName: _.get(selectedNode, 'name'),
    });
  }

  componentWillReceiveProps = () => {
    const selectedNode = this.getSelectedNode();

    if (!_.isEqual(selectedNode, this.state.selectedNode)) {
      this.setState({
        selectedNode,
        selectedNodeName: _.get(selectedNode, 'name'),
      });
    }
  }

  getSelectedNode() {
    const { getSelectedNode } = this.context;
    return getSelectedNode();
  }

  handlePutNodeChange = (e: any) => {
    const { value } = e.target;
    this.setState({ selectedNodeName: value });
  }

  handleNewNodeLeafChange = (e: any) => {
    this.setState({ newNodeLeaf: e.target.checked });
  }

  handlePutNode = () => {
    const { reloadNsTree, updateSelectedNode } = this.context;
    const { selectedNode, selectedNodeName } = this.state;
    if (selectedNode && selectedNodeName) {
      request(`${api.node}/${selectedNode.id}/name`, {
        method: 'PUT',
        body: JSON.stringify({ name: selectedNodeName }),
      }).then(() => {
        reloadNsTree();
        updateSelectedNode({
          ...selectedNode,
          name: selectedNodeName,
          path: updatePathByName(selectedNode.path, selectedNodeName),
        });
        message.success('节点重命名成功！');
      });
    }
  }

  handleNewNodeNameChange = (e: any) => {
    const { value } = e.target;
    this.setState({ newNodeName: value });
  }

  handlePostNode = () => {
    const { reloadNsTree } = this.context;
    const { selectedNode, newNodeName, newNodeLeaf } = this.state;
    if (selectedNode) {
      request(api.node, {
        method: 'POST',
        body: JSON.stringify({
          pid: selectedNode.id,
          name: newNodeName,
          leaf: newNodeLeaf ? 1 : 0,
        }),
      }).then(() => {
        reloadNsTree();
        message.success('创建子节点成功！');
      });
    }
  }

  handleDelNode = () => {
    const { reloadNsTree, deleteSelectedNode } = this.context;
    const { selectedNode } = this.state;
    if (selectedNode) {
      request(`${api.node}/${selectedNode.id}`, {
        method: 'DELETE',
      }).then(() => {
        reloadNsTree();
        deleteSelectedNode();
        message.success('节点删除成功！');
      });
    }
  }

  render() {
    const { selectedNode, selectedNodeName, newNodeName, newNodeLeaf } = this.state;
    const isPdlNode = _.get(selectedNode, 'pid') === 0;
    const isLeafNode = _.get(selectedNode, 'leaf') === 1;
    if (!selectedNode) {
      return (
        <div>
          请先选择左侧服务节点
        </div>
      );
    }
    return (
      <div>
        <Row gutter={20}>
          <Col span={8} className="mb10">
            节点重命名：
            <div className="mt10 mb10">
              <Input
                style={{ width: 200 }}
                value={selectedNodeName}
                onChange={this.handlePutNodeChange}
                placeholder="新节点名称"
              />
            </div>
            <Button onClick={this.handlePutNode}>保存</Button>
          </Col>
          <Col span={8} className="mb10">
            创建子节点：
            <div className="mt10 mb10">
              <Input
                style={{ width: 200 }}
                value={newNodeName}
                onChange={this.handleNewNodeNameChange}
                placeholder="子节点名称"
                disabled={isLeafNode}
              />
            </div>
            <div className="mt10 mb10">
              <Checkbox
                checked={newNodeLeaf}
                onChange={this.handleNewNodeLeafChange}
                disabled={isLeafNode}
              >
                是否叶子节点
              </Checkbox>
            </div>
            <Button disabled={isLeafNode} onClick={this.handlePostNode}>创建</Button>
            {
              isLeafNode ? <p className="fc50 mt10">叶子节点无法继续创建子节点</p> : null
            }
          </Col>
          <Col span={8} className="mb10">
            删除该节点：
            <div className="mt10 mb10" style={{ wordBreak: 'break-word' }}>
              {_.get(selectedNode, 'path')}
            </div>
            <Popconfirm title="确定要删除这个节点吗？" onConfirm={this.handleDelNode}>
              <Button disabled={isPdlNode}>删除</Button>
            </Popconfirm>
            {
              isPdlNode ? <p className="fc50 mt10">{config.aliasMap.dept}节点不能删除</p> : null
            }
          </Col>
        </Row>
      </div>
    );
  }
}

export default CreateIncludeNsTree(index, { visible: true });
