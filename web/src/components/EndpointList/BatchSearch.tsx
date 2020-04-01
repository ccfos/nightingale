import React, { Component } from 'react';
import { Modal, Form, Input, Radio } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { FormattedMessage } from 'react-intl';

interface Props {
  field: string,
  batch: string,
  title: string,
  visible: boolean,
  onOk: (field: string, batch: string) => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;
const RadioGroup = Radio.Group;

class BatchSearch extends Component<Props & FormProps> {
  static defaultProps: any = {
    field: 'ident',
    batch: '',
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    this.props.form!.validateFields((err, values) => {
      if (!err) {
        const batch = _.replace(values.batch, /\n/g, ',');
        this.props.onOk(values.field, batch);
        this.props.destroy();
      }
    });
  }

  handleCancel = () => {
    this.props.destroy();
  }

  render() {
    const { title, visible, field, batch } = this.props;
    const { getFieldDecorator } = this.props.form!;

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form layout="vertical">
          <FormItem label={<FormattedMessage id="endpoints.batch.filter.key" />}>
            {getFieldDecorator('field', {
              initialValue: field,
            })(
              <RadioGroup>
                <Radio value="ident"><FormattedMessage id="endpoints.ident" /></Radio>
                <Radio value="alias"><FormattedMessage id="endpoints.alias" /></Radio>
              </RadioGroup>,
            )}
          </FormItem>
          <FormItem label={<FormattedMessage id="endpoints.batch.filter.value" />}>
            {getFieldDecorator('batch', {
              initialValue: _.replace(batch, /,/g, '\n'),
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

export default ModalControl(Form.create()(BatchSearch));
