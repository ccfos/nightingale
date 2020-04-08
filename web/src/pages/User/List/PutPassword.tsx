import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
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

class PutPassword extends Component<Props & FormProps & WrappedComponentProps> {
  static defaultProps = {
    title: '',
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
          message.success(this.props.intl.formatMessage({ id: 'user.reset.password.success' }));
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
          <FormItem label={<FormattedMessage id="password.new" />} required>
            {getFieldDecorator('password', {
              rules: [{ required: true }],
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

export default ModalControl(Form.create()(injectIntl(PutPassword)));
