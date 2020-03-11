import React, { Component, FormEvent } from 'react';
import { RouteComponentProps } from 'react-router-dom';
import { Card, Button, message } from 'antd';
import queryString from 'query-string';
import ProfileForm from '@cpts/ProfileForm';
import request from '@common/request';
import api from '@common/api';
import { appname } from '@common/config';
import './style.less';

class Register extends Component<RouteComponentProps> {
  profileForm: any; // TODO useRef
  handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const { location, history } = this.props;
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
          message.success('注册成功！');
          history.push({
            pathname: '/',
          });
        });
      }
    });
  }

  render() {
    const prefixCls = `${appname}-register`;

    return (
      <div className={prefixCls}>
        <div className={`${prefixCls}-main`}>
          <Card>
            <div className={`${prefixCls}-title`}>账户注册</div>
            <ProfileForm type="register" ref={(ref: any) => { this.profileForm = ref; }} />
            <Button
              type="primary"
              className={`${prefixCls}-submitBtn`}
              onClick={this.handleSubmit}
            >
              注 册
            </Button>
          </Card>
        </div>
      </div>
    );
  }
}

export default Register;
