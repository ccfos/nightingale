import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { withRouter, RouteComponentProps } from 'react-router-dom';
import { Spin, message } from 'antd';
import PropTypes from 'prop-types';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import { normalizeTreeData } from '@cpts/Layout/utils';
import request from '@common/request';
import api from '@common/api';
import CollectForm from './CollectForm';

class CollectFormMain extends Component<RouteComponentProps & WrappedComponentProps> {
  static contextTypes = {
    getSelectedNode: PropTypes.func,
  };
  selectedNodeId: number | undefined = undefined;
  state = {
    loading: false,
    data: {} as any,
    selectedTreeNode: {},
    treeData: [],
  };

  componentWillMount = () => {
    const { getSelectedNode } = this.context;
    this.selectedNodeId = getSelectedNode('id');
  }

  componentDidMount() {
    this.fetchTreeData();
    this.fetchData();
  }

  fetchTreeData() {
    request(api.tree).then((res) => {
      const treeData = normalizeTreeData(res);
      this.setState({ treeData });
    });
  }

  fetchData = () => {
    const params = _.get(this.props, 'match.params');
    if (params.action !== 'add') {
      this.setState({ loading: true });
      request(`${api.collect}?id=${params.id}&type=${params.type}`).then((res) => {
        this.setState({
          data: res || {},
        });
      }).finally(() => {
        this.setState({ loading: false });
      });
    }
  }

  handleSubmit = (values: any) => {
    const { action, type } = this.props.match.params as any;
    let reqBody;

    if (action === 'add' || action === 'clone') {
      reqBody = [{
        type,
        data: values,
      }];
    } else if (action === 'modify') {
      reqBody = {
        type,
        data: {
          ...values,
          id: this.state.data.id,
        },
      };
    }

    return request(api.collect, {
      method: action === 'modify' ? 'PUT' : 'POST',
      body: JSON.stringify(reqBody),
    }).then(() => {
      message.success(this.props.intl.formatMessage({ id: 'msg.submit.success' }));
      this.props.history.push({
        pathname: '/monitor/collect',
      });
    });
  }

  render() {
    const { action, type } = this.props.match.params as any;
    const { treeData, data, loading } = this.state;
    const ActiveForm = CollectForm[type];
    if (action === 'add') {
      data.nid = this.selectedNodeId;
    }

    return (
      <Spin spinning={loading}>
        <ActiveForm
          params={this.props.match.params}
          treeData={treeData}
          initialValues={data}
          onSubmit={this.handleSubmit}
        />
      </Spin>
    );
  }
}

export default CreateIncludeNsTree(withRouter(injectIntl(CollectFormMain)));
