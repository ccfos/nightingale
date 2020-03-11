import React, { Component } from 'react';
import { Form, Input, Icon } from 'antd';
import { FormProps } from 'antd/lib/form';

const FormItem = Form.Item;

class index extends Component<FormProps> {
  validateFields() {
    return this.props.form!.validateFields;
  }

  render() {
    const { getFieldDecorator } = this.props.form!;
    return (
      <Form layout="vertical">
        <FormItem label="旧密码" required>
          {getFieldDecorator('oldpass', {
            rules: [{ required: true, message: '请输入旧密码!' }],
          })(
            <Input
              prefix={<Icon type="lock" style={{ color: 'rgba(0,0,0,.25)' }} />}
              type="password"
            />,
          )}
        </FormItem>
        <FormItem label="新密码" required>
          {getFieldDecorator('newpass', {
            rules: [{ required: true, message: '请输入新密码!' }],
          })(
            <Input
              prefix={<Icon type="lock" style={{ color: 'rgba(0,0,0,.25)' }} />}
              type="password"
            />,
          )}
        </FormItem>
      </Form>
    );
  }
}

export default Form.create()(index);
