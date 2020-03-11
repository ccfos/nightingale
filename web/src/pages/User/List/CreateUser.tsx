import React, { Component } from 'react';
import { Modal, message } from 'antd';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import ProfileForm from '@cpts/ProfileForm';
import { auth } from '@cpts/Auth';
import request from '@common/request';
import api from '@common/api';

interface Props {
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

class CreateUser extends Component<Props> {
  profileFormRef: any;
  static defaultProps = {
    title: '新建用户',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    this.profileFormRef.validateFields((err: any, values: any) => {
      if (!err) {
        request(api.user, {
          method: 'POST',
          body: JSON.stringify({
            ...values,
            is_root: values.is_root ? 1 : 0,
          }),
        }).then(() => {
          message.success('新建用户成功！');
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
    const { isroot } = auth.getSelftProfile();

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <ProfileForm
          isrootVsible={isroot}
          ref={(ref: any) => { this.profileFormRef = ref; }} />
      </Modal>
    );
  }
}

export default ModalControl(CreateUser);
