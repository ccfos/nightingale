import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps } from 'react-intl';
import { Modal, message } from 'antd';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import ProfileForm from '@cpts/ProfileForm';
import { auth } from '@cpts/Auth';
import request from '@common/request';
import api from '@common/api';
import { UserProfile } from '@interface';

interface Props {
  data: UserProfile,
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

class PutProfile extends Component<Props & WrappedComponentProps> {
  profileForm: any;

  static defaultProps = {
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    this.profileForm.validateFields((err: any, values: any) => {
      if (!err) {
        request(`${api.user}/${this.props.data.id}/profile`, {
          method: 'PUT',
          body: JSON.stringify({
            ...values,
            is_root: values.is_root ? 1 : 0,
          }),
        }).then(() => {
          message.success(this.props.intl.formatMessage({ id: 'msg.modify.success' }));
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
    const { title, visible, data } = this.props;
    const { isroot } = auth.getSelftProfile();

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <ProfileForm
          type="put"
          isrootVsible={isroot}
          initialValue={data}
          ref={(ref: any) => { this.profileForm = ref; }}
        />
      </Modal>
    );
  }
}

export default ModalControl(injectIntl(PutProfile));
