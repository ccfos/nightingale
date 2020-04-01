import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import ReactDOM from 'react-dom';
import { Modal } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import CustomForm from './CustomForm';

interface Props extends FormProps {
  category: string,
  data: any,
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

class DetailModal extends Component<Props> {
  static defaultProps = {
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };
  customForm: any;
  state = {
    submitLoading: false,
  };

  handleOk = () => {
    this.props.onOk();
    this.props.destroy();
  }

  handleCancel = () => {
    this.props.onCancel();
    this.props.destroy();
  }

  render() {
    const {
      title, visible, category, data,
    } = this.props;
    const { submitLoading } = this.state;

    return (
      <div>
        <Modal
          width={900}
          title={<FormattedMessage id="silence.detail.title" />}
          visible={visible}
          onOk={this.handleOk}
          onCancel={this.handleCancel}
          confirmLoading={submitLoading}
        >
          <CustomForm
            ref={(ref) => { this.customForm = ref; }}
            category={category}
            initialValues={data}
            readOnly
          />
        </Modal>
      </div>
    );
  }
}

export default function detailModal(config: any) {
  const div = document.createElement('div');
  document.body.appendChild(div);

  function destroy() {
    const unmountResult = ReactDOM.unmountComponentAtNode(div);
    if (unmountResult && div.parentNode) {
      div.parentNode.removeChild(div);
    }
  }

  function render(props: any) {
    ReactDOM.render(<DetailModal {...props} />, div);
  }

  render({ ...config, visible: true, destroy });

  return {
    destroy,
  };
}
