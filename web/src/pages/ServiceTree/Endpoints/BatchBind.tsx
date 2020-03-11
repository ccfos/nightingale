import React, { Component } from 'react';
import { Modal, Form, Input, Checkbox, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { TreeNode } from '@interface';
import request from '@common/request';
import api from '@common/api';

interface Props {
  selectedNode: TreeNode,
  title?: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;

class BatchBind extends Component<Props & FormProps> {
  static defaultProps = {
    title: '挂载 endpoints',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    const { selectedNode } = this.props;
    this.props.form!.validateFields((err, values) => {
      if (!err) {
        const reqBody = {
          idents: _.split(values.idents, '\n'),
          del_old: values.del_old ? 1 : 0,
        };
        request( `${api.node}/${selectedNode.id}/endpoint-bind`, {
          method: 'POST',
          body: JSON.stringify(reqBody),
        }).then(() => {
          message.success('挂载成功！');
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
    const { title, visible, selectedNode } = this.props;
    const { getFieldDecorator } = this.props.form!;

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form layout="vertical">
          <FormItem label="挂载的节点">
            <span className="ant-form-text" style={{ wordBreak: 'break-word' }}>{_.get(selectedNode, 'path')}</span>
          </FormItem>
          <FormItem label="待挂载的 endpoint">
            {getFieldDecorator('idents', {
              rules: [{ required: true, message: '请填写需要挂载的 endpoints!' }],
            })(
              <Input.TextArea
                autosize={{ minRows: 2, maxRows: 10 }}
              />,
            )}
          </FormItem>
          {getFieldDecorator('del_old', {
          })(
            <Checkbox className="mt10">是否删除旧的挂载关系</Checkbox>,
          )}
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(BatchBind));
