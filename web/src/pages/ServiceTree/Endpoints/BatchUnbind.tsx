import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { Modal, Form, Input, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
import ModalControl from '@cpts/ModalControl';
import { TreeNode } from '@interface';
import request from '@common/request';
import api from '@common/api';

interface Props {
  selectedNode: TreeNode,
  selectedIdents: string[],
  title?: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;

class BatchHostUnbind extends Component<Props & FormProps & WrappedComponentProps> {
  static defaultProps = {
    title: '',
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
        };
        request(`${api.node}/${selectedNode.id}/endpoint-unbind`, {
          method: 'POST',
          body: JSON.stringify(reqBody),
        }).then(() => {
          message.success(this.props.intl.formatMessage({ id: 'msg.submit.success' }));
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
    const { title, visible, selectedNode, selectedIdents } = this.props;
    const { getFieldDecorator } = this.props.form!;

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form layout="vertical">
          <FormItem label={<FormattedMessage id="endpoints.unbind.node" />}>
            <span className="ant-form-text" style={{ wordBreak: 'break-word' }}>{_.get(selectedNode, 'path')}</span>
          </FormItem>
          <FormItem label={<span>Endpoints <FormattedMessage id="endpoints.ident" /></span>}>
            {getFieldDecorator('idents', {
              initialValue: _.join(selectedIdents, '\n'),
              rules: [{ required: true }],
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

export default ModalControl(Form.create()(injectIntl(BatchHostUnbind)));
