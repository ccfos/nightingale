import React, { Component }from 'react';
import PropTypes from 'prop-types';
import { Modal, message } from 'antd';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { Team } from '@interface';
import request from '@common/request';
import api from '@common/api';
import TeamForm from './TeamForm';

interface Props {
  data: Team,
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

class PutTeam extends Component<Props> {
  teamFormRef: any;
  static propTypes = {
    data: PropTypes.object.isRequired,
    title: PropTypes.string,
    visible: PropTypes.bool,
    onOk: PropTypes.func,
    onCancel: PropTypes.func,
    destroy: PropTypes.func,
  };

  static defaultProps = {
    title: '编辑团队',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  handleOk = () => {
    const { data } = this.props;
    this.teamFormRef.validateFields((err: any, values: any) => {
      if (!err) {
        request(`${api.team}/${data.id}`, {
          method: 'PUT',
          body: JSON.stringify({
            ...values,
          }),
        }).then(() => {
          message.success('团队信息修改成功！');
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

    return (
      <Modal
        title={title}
        visible={visible}
        onOk={this.handleOk}
        onCancel={this.handleCancel}
      >
        <TeamForm
          initialValue={data}
          ref={(ref: any) => { this.teamFormRef = ref; }}
        />
      </Modal>
    );
  }
}

export default ModalControl(PutTeam);
