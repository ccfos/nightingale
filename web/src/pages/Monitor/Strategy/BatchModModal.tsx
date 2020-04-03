import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Modal, Form, Select, TreeSelect, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { normalizeTreeData, renderTreeNodes, filterTreeNodes } from '@cpts/Layout/utils';
import request from '@common/request';
import api from '@common/api';

interface Props extends FormProps{
  data: any[], // 批量操作的数据
  type: string, // exclNid 排除节点，notify 报警接收人
  selectedNid: number,
  treeNodes: any[],
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;
const { Option } = Select;

class BatchModModal extends Component<Props & WrappedComponentProps> {
  static defaultProps = {
    selectedNid: undefined,
    treeNodes: [],
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  state = {
    loading: false,
    treeData: [],
    excludeTreeData: [],
    notifyGroupData: [],
    notifyUserData: [],
  };

  componentDidMount = () => {
    if (this.props.type === 'exclNid' || this.props.type === 'clone') {
      const treeData = normalizeTreeData(_.cloneDeep(this.props.treeNodes));
      const excludeTreeData = filterTreeNodes(treeData, this.props.selectedNid);
      this.setState({ treeData, excludeTreeData });
    }
    if (this.props.type === 'notify') {
      this.fetchNotifyData();
    }
  }

  async fetchNotifyData() {
    try {
      const teamData = await request(`${api.team}?limit=1000`);
      const userData = await request(`${api.user}?limit=1000`);
      this.setState({
        notifyGroupData: teamData.list,
        notifyUserData: userData.list,
      });
    } catch (e) {
      console.log(e);
    }
  }

  handleOk = () => {
    this.props.form!.validateFields(async (err, values) => {
      if (!err) {
        this.setState({ loading: true });
        try {
          const requests = _.map(this.props.data, (item) => {
            if (this.props.type === 'clone') {
              delete item.id;
              delete item.excl_nid;
            }
            request(api.stra, {
              method: this.props.type === 'clone' ? 'POST' : 'PUT',
              body: JSON.stringify({
                ...item,
                ...values,
              }),
            });
          });
          await Promise.all(requests).then(() => {
            message.success(this.props.intl.formatMessage({ id: 'msg.modify.success' }));
          }).catch(() => {
            // message.error('批量操作失败！');
          });
        } catch (e) {
          console.log(e);
        }
        this.setState({ loading: false });
        this.props.onOk();
        this.props.destroy();
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
        confirmLoading={this.state.loading}
      >
        <Form layout="vertical">
          {
            this.props.type === 'exclNid' ?
              <FormItem
                label={<FormattedMessage id="stra.node.exclude" />}
              >
                {
                  getFieldDecorator('excl_nid', {
                    // initialValue: this.props.initialValues.excl_nid,
                  })(
                    <TreeSelect
                      multiple
                      showSearch
                      allowClear
                      treeDefaultExpandAll
                      treeNodeFilterProp="title"
                      treeNodeLabelProp="path"
                      dropdownStyle={{ maxHeight: 400, overflow: 'auto' }}
                    >
                      {renderTreeNodes(this.state.excludeTreeData)}
                    </TreeSelect>,
                  )
                }
              </FormItem> : null
          }
          {
            this.props.type === 'notify' ?
              [
                <FormItem
                  key="group"
                  label={<FormattedMessage id="stra.notify.team" />}
                >
                  {
                    getFieldDecorator('notify_group', {
                      initialValue: [],
                    })(
                      <Select
                        mode="multiple"
                        size="default"
                        defaultActiveFirstOption={false}
                        filterOption={false}
                      >
                        {
                          _.map(this.state.notifyGroupData, (item: any, i) => {
                            return (
                              <Option key={i} value={item.id}>{item.name}</Option>
                            );
                          })
                        }
                      </Select>,
                    )
                  }
                </FormItem>,
                <FormItem
                  key="user"
                  label={<FormattedMessage id="stra.notify.user" />}
                >
                  {
                    getFieldDecorator('notify_user', {
                      initialValue: [],
                    })(
                      <Select
                        mode="multiple"
                        size="default"
                        defaultActiveFirstOption={false}
                        filterOption={false}
                      >
                        {
                          _.map(this.state.notifyUserData, (item: any, i) => {
                            return (
                              <Option key={i} value={item.id}>{item.username} {item.dispname} {item.phone} {item.email}</Option>
                            );
                          })
                        }
                      </Select>,
                    )
                  }
                </FormItem>,
              ] : null
          }
          {
            this.props.type === 'clone' ?
              <FormItem
                label={<FormattedMessage id="stra.node" />}
              >
                {
                  getFieldDecorator('nid', {
                  })(
                    <TreeSelect
                      showSearch
                      allowClear
                      treeDefaultExpandAll
                      treeNodeFilterProp="title"
                      treeNodeLabelProp="path"
                      dropdownStyle={{ maxHeight: 400, overflow: 'auto' }}
                    >
                      {renderTreeNodes(this.state.treeData)}
                    </TreeSelect>,
                  )
                }
              </FormItem> : null
          }
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(injectIntl(BatchModModal)));
