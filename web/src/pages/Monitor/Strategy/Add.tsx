import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { RouteComponentProps } from 'react-router-dom';
import { message } from 'antd';
import queryString from 'query-string';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import SettingFields from './SettingFields';
import './style.less';

class Add extends Component<RouteComponentProps & WrappedComponentProps> {
  handleSubmit = (values: any) => {
    const { history } = this.props;
    request(api.stra, {
      method: 'POST',
      body: JSON.stringify(values),
    }).then(() => {
      message.success(this.props.intl.formatMessage({ id: 'msg.add.success' }));
      history.push({
        pathname: '/monitor/strategy',
      });
    });
  }

  render() {
    const search = _.get(this.props, 'location.search');
    const query = queryString.parse(search);
    const nid = _.toNumber(query.nid);
    return (
      <div>
        <SettingFields
          onSubmit={this.handleSubmit}
          initialValues={{
            nid,
          }}
        />
      </div>
    );
  }
}

export default CreateIncludeNsTree(injectIntl(Add));
