import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { RouteComponentProps } from 'react-router-dom';
import { message } from 'antd';
import _ from 'lodash';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import SettingFields from './SettingFields';
import { normalizeFormData } from './utils';
import './style.less';

class Modify extends Component<RouteComponentProps & WrappedComponentProps> {
  state = {
    values: undefined,
  } as { values: any };

  componentDidMount = () => {
    this.getStrategy(this.props);
  }

  getStrategy(props: RouteComponentProps) {
    const strategyId = _.get(props, 'match.params.strategyId');
    if (strategyId) {
      request(`${api.stra}/${strategyId}`).then((values) => {
        this.setState({
          values: normalizeFormData(values),
        });
      });
    }
  }

  handleSubmit = (newValues: any) => {
    const { history } = this.props;
    const { values } = this.state;
    request(api.stra, {
      method: 'PUT',
      body: JSON.stringify({
        ...newValues,
        id: values.id,
      }),
    }).then(() => {
      message.success(this.props.intl.formatMessage({ id: 'msg.modify.success' }));
      history.push({
        pathname: '/monitor/strategy',
      });
    });
  }

  render() {
    const { values } = this.state;
    if (values) {
      return (
        <div>
          <SettingFields
            initialValues={values}
            onSubmit={this.handleSubmit}
          />
        </div>
      );
    }
    return null;
  }
}

export default CreateIncludeNsTree(injectIntl(Modify));
