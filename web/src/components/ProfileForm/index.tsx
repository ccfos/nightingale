import React, { Component, Fragment } from 'react';
import { Form, Input, Switch, Icon } from 'antd';
import { FormProps } from 'antd/lib/form';
import { UserProfile } from '@interface';

interface Props {
  type: string,
  isrootVsible: boolean,
  initialValue: UserProfile,
}

const FormItem = Form.Item;

class ProfileForm extends Component<Props & FormProps> {
  static defaultProps = {
    type: 'post',
    isrootVsible: false,
    initialValue: {},
  };

  validateFields() {
    return this.props.form!.validateFields;
  }

  renderLabel(name: string) {
    const { type } = this.props;
    if (type === 'register') {
      return '';
    }
    return name;
  }

  render() {
    const { type, isrootVsible, initialValue } = this.props;
    const { getFieldDecorator } = this.props.form!;
    return (
      <Form layout="vertical">
        {
          type === 'post' || type === 'register' ?
            <Fragment>
              <FormItem label={this.renderLabel('用户名')} required>
                {getFieldDecorator('username', {
                  rules: [{ required: true, message: '请输入用户名!' }],
                })(
                  <Input placeholder="用户名" />,
                )}
              </FormItem>
              <FormItem label={this.renderLabel('密码')} required>
                {getFieldDecorator('password', {
                  rules: [{ required: true, message: '请输入密码!' }],
                })(
                  <Input type="password" placeholder="密码" />,
                )}
              </FormItem>
            </Fragment> : null
        }
        <FormItem label={this.renderLabel('显示名')} required>
          {getFieldDecorator('dispname', {
            initialValue: initialValue.dispname,
            rules: [{ required: true, message: '请输入显示名!' }],
          })(
            <Input placeholder="显示名" />,
          )}
        </FormItem>
        <FormItem label={this.renderLabel('手机')}>
          {getFieldDecorator('phone', {
            initialValue: initialValue.phone,
          })(
            <Input placeholder="手机" style={{ width: '100%' }} />,
          )}
        </FormItem>
        <FormItem label={this.renderLabel('邮箱')}>
          {getFieldDecorator('email', {
            initialValue: initialValue.email,
          })(
            <Input placeholder="邮箱" />,
          )}
        </FormItem>
        <FormItem label={this.renderLabel('im')}>
          {getFieldDecorator('im', {
            initialValue: initialValue.im,
          })(
            <Input placeholder="im" />,
          )}
        </FormItem>
        {
          isrootVsible ?
            <FormItem label={this.renderLabel('是否超管')}>
              {getFieldDecorator('is_root', {
                valuePropName: 'checked',
                initialValue: initialValue.is_root === 1,
              })(
                <Switch
                  checkedChildren={<Icon type="check" />}
                  unCheckedChildren={<Icon type="close" />}
                />,
              )}
            </FormItem> : null
        }
      </Form>
    );
  }
}

export default Form.create()(ProfileForm as any);
