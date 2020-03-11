import React, { Component } from 'react';
import { Modal, Form, Input, Icon, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import request from '@common/request';
import api from '@common/api';

interface Props {
  id: number,
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;

class PutPassword extends Component<Props & FormProps> {
  static defaultProps = {
    title: '修改密码',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    this.props.form!.validateFields((err, values) => {
      if (!err) {
        request(`${api.user}/${this.props.id}/password`, {
          method: 'PUT',
          body: JSON.stringify(values),
        }).then(() => {
          message.success('密码修改成功！');
          this.props.onOk();
          this.props.destroy();
        });
      }
    });
  }

  handleCancel = () => {
    this.props.destroy();
  }

  render() {
    const { title, visible } = this.props;
    const { getFieldDecorator } = this.props.form!;

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form layout="vertical">
          <FormItem label="新密码" required>
            {getFieldDecorator('password', {
              rules: [{ required: true, message: '请输入新密码!' }],
            })(
              <Input
                prefix={<Icon type="lock" style={{ color: 'rgba(0,0,0,.25)' }} />}
                type="password"
              />,
            )}
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(PutPassword as any));
