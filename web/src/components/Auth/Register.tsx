import React, { Component, FormEvent } from 'react';
import { RouteComponentProps } from 'react-router-dom';
import { Card, Button, message } from 'antd';
import queryString from 'query-string';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import ProfileForm from '@cpts/ProfileForm';
import request from '@common/request';
import api from '@common/api';
import { appname } from '@common/config';
import './style.less';

class Register extends Component<RouteComponentProps & WrappedComponentProps> {
  profileForm: any; // TODO useRef
  handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const { location, history } = this.props;
    const { formatMessage } = this.props.intl;
    const query = queryString.parse(location.search);
    this.profileForm.validateFields((err: any, values: any) => {
      if (!err) {
        request(`${api.users}/invite`, {
          method: 'POST',
          body: JSON.stringify({
            ...values,
            token: query.token,
          }),
        }).then(() => {
          message.success(formatMessage({ id: 'msg.submit.success' }));
          history.push({
            pathname: '/',
          });
        });
      }
    });
  }

  render() {
    const prefixCls = `${appname}-register`;
    const { formatMessage } = this.props.intl;

    return (
      <div className={prefixCls}>
        <div className={`${prefixCls}-main`}>
          <Card>
            <div className={`${prefixCls}-title`}>{formatMessage({ id: 'register' })}</div>
            <ProfileForm type="register" ref={(ref: any) => { this.profileForm = ref; }} />
            <Button
              type="primary"
              className={`${prefixCls}-submitBtn`}
              onClick={this.handleSubmit}
            >
              {formatMessage({ id: 'register' })}
            </Button>
          </Card>
        </div>
      </div>
    );
  }
}

export default injectIntl(Register);
