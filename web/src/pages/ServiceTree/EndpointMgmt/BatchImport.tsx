import React, { Component }from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { Modal, Form, Input, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import { FormattedMessage } from 'react-intl';
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

class BatchImport extends Component<Props & FormProps & WrappedComponentProps> {
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
        request(api.endpoint, {
          method: 'POST',
          body: JSON.stringify({
            endpoints: _.split(values.endpoints, '\n'),
          }),
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
            label="Endpoints"
            help={<FormattedMessage id="endpoints.import.batch.help" />}
          >
            {getFieldDecorator('endpoints', {
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

export default ModalControl(Form.create()(injectIntl(BatchImport)));
