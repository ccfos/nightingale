import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { Modal, Form, Input, Checkbox, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
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

class BatchBind extends Component<Props & FormProps & WrappedComponentProps> {
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
          del_old: values.del_old ? 1 : 0,
        };
        request( `${api.node}/${selectedNode.id}/endpoint-bind`, {
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
          <FormItem label={<FormattedMessage id="endpoints.bind.node" />}>
            <span className="ant-form-text" style={{ wordBreak: 'break-word' }}>{_.get(selectedNode, 'path')}</span>
          </FormItem>
          <FormItem label={<span>Endpoints <FormattedMessage id="endpoints.ident" /></span>}>
            {getFieldDecorator('idents', {
              rules: [{ required: true }],
            })(
              <Input.TextArea
                autosize={{ minRows: 2, maxRows: 10 }}
              />,
            )}
          </FormItem>
          {getFieldDecorator('del_old', {
          })(
            <Checkbox className="mt10"><FormattedMessage id="endpoints.delete.old.bind" /></Checkbox>,
          )}
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(injectIntl(BatchBind)));
