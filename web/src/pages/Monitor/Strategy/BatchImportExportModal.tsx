import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { Modal, Form, Input, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import request from '@common/request';
import api from '@common/api';

interface Props extends FormProps{
  data: any[], // 批量操作的数据
  type: string, // import | export
  selectedNid: number,
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;
const { TextArea } = Input;

class BatchImportExportModal extends Component<Props & WrappedComponentProps> {
  static defaultProps = {
    data: undefined,
    selectedNid: undefined,
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    if (this.props.type === 'import') {
      const { getFieldValue } = this.props.form!;
      const data = getFieldValue('data');
      let parsed;

      try {
        parsed = _.map(JSON.parse(data), (item) => {
          return {
            ...item,
            nid: this.props.selectedNid,
          };
        });
      } catch (e) {
        console.log(e);
      }

      const promises = _.map(parsed, (item) => {
        return request(api.stra, {
          method: 'POST',
          body: JSON.stringify(item),
        });
      });
      Promise.all(promises).then(() => {
        message.success(this.props.intl.formatMessage({ id: 'stra.batch.import.success' }));
        this.props.onOk();
        this.props.destroy();
      });
    } else {
      this.props.destroy();
    }
  }

  handleCancel = () => {
    this.props.destroy();
  }

  render() {
    const { title, visible, data } = this.props;
    const { getFieldDecorator } = this.props.form!;
    let initialValue;

    try {
      initialValue = !_.isEmpty(data) ? JSON.stringify(data, null, 4) : undefined;
    } catch (e) {
      console.log(e);
    }

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <Form layout="vertical">
          <FormItem>
            {
              getFieldDecorator('data', {
                initialValue,
              })(
                <TextArea autosize={{ minRows: 2, maxRows: 10 }} />,
              )
            }
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(injectIntl(BatchImportExportModal)));
