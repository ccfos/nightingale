import React, { Component, FormEvent } from 'react';
import { RouteComponentProps } from 'react-router-dom';
import { Card, Form, Input, Icon, Button, Checkbox } from 'antd';
import { FormProps } from 'antd/lib/form';
import queryString from 'query-string';
import _ from 'lodash';
import { appname } from '@common/config';
import auth from './auth';
import './style.less';

const FormItem = Form.Item;

class Login extends Component<RouteComponentProps & FormProps> {
  handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const { history, location } = this.props;
    const { search } = location;

    this.props.form!.validateFields((err, values) => {
      if (!err) {
        auth.authenticate({
          ...values,
          is_ldap: values.is_ldap ? 1 : 0,
        }, () => {
          const query = queryString.parse(search);
          const locationState = location.state as { from: string };
          if (query.callback && query.sig) {
            if (query.callback.indexOf('?') > -1) {
              window.location.href = `${query.callback}&sig=${query.sig}`;
            } else {
              window.location.href = `${query.callback}?sig=${query.sig}`;
            }
          } else if (_.findKey(locationState, 'from')) {
            history.push(locationState.from);
          } else {
            history.push({
              pathname: '/',
            });
          }
        });
      }
    });
  }

  render() {
    const prefixCls = `${appname}-login`;
    const { history } = this.props;
    const { getFieldDecorator } = this.props.form!;
    const isAuthenticated = auth.getIsAuthenticated();

    if (isAuthenticated) {
      history.push({
        pathname: '/',
      });
      return null;
    }
    return (
      <div className={prefixCls}>
        <div className={`${prefixCls}-main`}>
          <Card>
            <div className={`${prefixCls}-title`}>账户登录</div>
            <Form onSubmit={this.handleSubmit}>
              <FormItem>
                {getFieldDecorator('username', {
                  rules: [{ required: true, message: '请输入你的用户名!' }],
                })(
                  <Input prefix={<Icon type="user" style={{ color: 'rgba(0,0,0,.25)' }} />} placeholder="用户名" />,
                )}
              </FormItem>
              <FormItem>
                {getFieldDecorator('password', {
                  rules: [{ required: true, message: '请输入你的密码!' }],
                })(
                  <Input prefix={<Icon type="lock" style={{ color: 'rgba(0,0,0,.25)' }} />} type="password" placeholder="密码" />,
                )}
              </FormItem>
              <FormItem>
                {getFieldDecorator('is_ldap', {
                  valuePropName: 'checked',
                  initialValue: false,
                })(
                  <Checkbox>使用LDAP账号登录</Checkbox>,
                )}
                <Button type="primary" htmlType="submit" className={`${prefixCls}-submitBtn`}>
                  登 录
                </Button>
              </FormItem>
            </Form>
          </Card>
        </div>
      </div>
    );
  }
}

export default Form.create()(Login);
