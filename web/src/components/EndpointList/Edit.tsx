import React, { Component } from 'react';
import { Modal, Form, Input, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { Endpoint } from '@interface';
import request from '@common/request';
import api from '@common/api';

interface Props {
  title?: string,
  data: Endpoint,
  titile: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;

class SingleEdit extends Component<FormProps & Props> {
  static defaultProps = {
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    const { title } = this.props;
    this.props.form!.validateFields((err, values) => {
      if (!err) {
        request(`${api.endpoint}/${values.id}`, {
          method: 'PUT',
          body: JSON.stringify({
            alias: values.alias,
          }),
        }).then(() => {
          message.success(`${title}成功`);
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
    const { title, visible, data } = this.props;
    const { getFieldDecorator } = this.props.form!;

    getFieldDecorator('id', {
      initialValue: data.id,
    });
    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form layout="vertical" onSubmit={(e) => {
          e.preventDefault();
          this.handleOk();
        }}>
          <FormItem label="标识">
            <span className="ant-form-text">{data.ident}</span>
          </FormItem>
          <FormItem label="别名">
            {getFieldDecorator('alias', {
              initialValue: data.alias,
            })(
              <Input />,
            )}
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(SingleEdit));
