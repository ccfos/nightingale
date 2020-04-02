import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { Modal, message } from 'antd';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import request from '@common/request';
import api from '@common/api';
import TeamForm from './TeamForm';

interface Props {
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

class AddTeam extends Component<Props & WrappedComponentProps> {
  teamFormRef: any;
  static defaultProps: any = {
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    this.teamFormRef.validateFields((err: any, values: any) => {
      if (!err) {
        request(api.team, {
          method: 'POST',
          body: JSON.stringify(values),
        }).then(() => {
          message.success(this.props.intl.formatMessage({ id: 'msg.create.success' }));
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

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <TeamForm
          ref={(ref: any) => { this.teamFormRef = ref; }}
        />
      </Modal>
    );
  }
}

export default ModalControl(injectIntl(AddTeam));
