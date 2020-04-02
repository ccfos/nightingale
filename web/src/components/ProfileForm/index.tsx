import React, { Component, Fragment } from 'react';
import { Form, Input, Switch, Icon } from 'antd';
import { FormProps } from 'antd/lib/form';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { UserProfile } from '@interface';

interface Props {
  type: string,
  isrootVsible: boolean,
  initialValue: UserProfile,
}

const FormItem = Form.Item;

class ProfileForm extends Component<Props & FormProps & WrappedComponentProps> {
  static defaultProps: any = {
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
    const { formatMessage } = this.props.intl;
    return (
      <Form layout="vertical">
        {
          type === 'post' || type === 'register' ?
            <Fragment>
              <FormItem label={this.renderLabel(formatMessage({ id: 'user.username' }))} required>
                {getFieldDecorator('username', {
                  rules: [{ required: true }],
                })(
                  <Input placeholder={formatMessage({ id: 'user.username' })} />,
                )}
              </FormItem>
              <FormItem label={this.renderLabel(formatMessage({ id: 'user.password' }))} required>
                {getFieldDecorator('password', {
                  rules: [{ required: true }],
                })(
                  <Input type="password" placeholder={formatMessage({ id: 'user.password' })} />,
                )}
              </FormItem>
            </Fragment> : null
        }
        <FormItem label={this.renderLabel(formatMessage({ id: 'user.dispname' }))} required>
          {getFieldDecorator('dispname', {
            initialValue: initialValue.dispname,
            rules: [{ required: true }],
          })(
            <Input placeholder={formatMessage({ id: 'user.dispname' })} />,
          )}
        </FormItem>
        <FormItem label={this.renderLabel(formatMessage({ id: 'user.phone' }))}>
          {getFieldDecorator('phone', {
            initialValue: initialValue.phone,
          })(
            <Input placeholder={formatMessage({ id: 'user.phone' })} style={{ width: '100%' }} />,
          )}
        </FormItem>
        <FormItem label={this.renderLabel(formatMessage({ id: 'user.email' }))}>
          {getFieldDecorator('email', {
            initialValue: initialValue.email,
          })(
            <Input placeholder={formatMessage({ id: 'user.email' })} />,
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
            <FormItem label={this.renderLabel(formatMessage({ id: 'user.isroot' }))}>
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

export default Form.create()(injectIntl(ProfileForm));
