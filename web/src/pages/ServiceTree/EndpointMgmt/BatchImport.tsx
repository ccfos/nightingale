import React, { Component }from 'react';
import { Modal, Form, Input, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import request from '@common/request';
import api from '@common/api';

interface Props {
  title?: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;

class BatchImport extends Component<Props & FormProps> {
  static defaultProps = {
    title: '批量导入',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    const { title } = this.props;
    this.props.form!.validateFields((err, values) => {
      if (!err) {
        request(api.endpoint, {
          method: 'POST',
          body: JSON.stringify({
            endpoints: _.split(values.endpoints, '\n'),
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
          <FormItem
            label="导入的 endpoints"
            help="每一条是 ident::alias 拼接在一起"
          >
            {getFieldDecorator('endpoints', {
              rules: [{ required: true, message: '请填写导入的机器 endpoints!' }],
            })(
              <Input.TextArea
                autosize={{ minRows: 2, maxRows: 10 }}
              />,
            )}
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(BatchImport));
