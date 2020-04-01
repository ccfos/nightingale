import React, { Component, FormEvent } from 'react';
import { RouteComponentProps } from 'react-router-dom';
import { Card, Form, Input, Icon, Button, Checkbox } from 'antd';
import { FormProps } from 'antd/lib/form';
import queryString from 'query-string';
import _ from 'lodash';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { appname } from '@common/config';
import auth from './auth';
import './style.less';

const FormItem = Form.Item;

class Login extends Component<RouteComponentProps & FormProps & WrappedComponentProps> {
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
    const { formatMessage } = this.props.intl;

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
            <div className={`${prefixCls}-title`}>{formatMessage({ id: 'login.title' })}</div>
            <Form onSubmit={this.handleSubmit}>
              <FormItem>
                {getFieldDecorator('username', {
                  rules: [{ required: true }],
                })(
                  <Input prefix={<Icon type="user" style={{ color: 'rgba(0,0,0,.25)' }} />} placeholder={formatMessage({ id: 'user.username' })} />,
                )}
              </FormItem>
              <FormItem>
                {getFieldDecorator('password', {
                  rules: [{ required: true }],
                })(
                  <Input prefix={<Icon type="lock" style={{ color: 'rgba(0,0,0,.25)' }} />} type="password" placeholder={formatMessage({ id: 'user.password' })} />,
                )}
              </FormItem>
              <FormItem>
                {getFieldDecorator('is_ldap', {
                  valuePropName: 'checked',
                  initialValue: false,
                })(
                  <Checkbox>{formatMessage({ id: 'login.ldap' })}</Checkbox>,
                )}
                <Button type="primary" htmlType="submit" className={`${prefixCls}-submitBtn`}>
                  {formatMessage({ id: 'form.login' })}
                </Button>
              </FormItem>
            </Form>
          </Card>
        </div>
      </div>
    );
  }
}

export default injectIntl(Form.create()(Login));
