import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Modal, Form, TreeSelect, Select, message } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { normalizeTreeData, renderTreeNodes } from '@cpts/Layout/utils';
import { TreeNode } from '@interface';
import request from '@common/request';
import api from '@common/api';
import { SubclassData } from './interface';

interface Props {
  configsList: any[],
  title: string,
  visible: boolean,
  onOk: () => void,
  onCancel: () => void,
  destroy: () => void,
}

interface State {
  treeData: TreeNode[],
  originTreeData: TreeNode[],
  screenData: any[],
  subclassData: SubclassData[],
}

const FormItem = Form.Item;
const { Option } = Select;
class SubscribeModal extends Component<Props & FormProps & WrappedComponentProps, State> {
  static defaultProps: any = {
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  state = {
    treeData: [],
    originTreeData: [],
    screenData: [],
    subclassData: [],
  } as State;

  componentDidMount() {
    this.fetchTreeData();
  }

  fetchTreeData() {
    request(api.tree).then((res) => {
      this.setState({ treeData: res });
      const treeData = normalizeTreeData(res);
      this.setState({ treeData, originTreeData: res });
    });
  }

  fetchScreenData() {
    const { getFieldValue } = this.props.form!;
    const nid = getFieldValue('nid');

    if (nid !== undefined) {
      request(`${api.node}/${nid}/screen`).then((res) => {
        this.setState({ screenData: res });
      });
    }
  }

  fetchSubclassData() {
    const { getFieldValue } = this.props.form!;
    const scrrenId = getFieldValue('scrrenId');

    if (scrrenId !== undefined) {
      request(`${api.screen}/${scrrenId}/subclass`).then((res) => {
        this.setState({ subclassData: res });
      });
    }
  }

  handleOk = () => {
    const { configsList } = this.props;
    this.props.form!.validateFields(async (err, values) => {
      if (!err) {
        try {
          const subclassChartData = await request(`${api.subclass}/${values.subclassId}/chart`);
          const startWeight = _.get(subclassChartData, 'length', 0);
          await Promise.all(
            _.map(configsList, (item, i) => {
              return request(`${api.subclass}/${values.subclassId}/chart`, {
                method: 'POST',
                body: JSON.stringify({
                  configs: item,
                  weight: startWeight + i,
                }),
              });
            }),
          );
          message.success(this.props.intl.formatMessage({ id: 'graph.subscribe.success' }));
          this.props.onOk();
          this.props.destroy();
        } catch (e) {
          console.log(e);
        }
      }
    });
  }

  handleCancel = () => {
    this.props.onCancel();
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
        bodyStyle={{ padding: 14 }}
        okText={<FormattedMessage id="graph.subscribe" />}
      >
        <Form layout="vertical" onSubmit={(e) => {
          e.preventDefault();
          this.handleOk();
        }}>
          <FormItem label={<FormattedMessage id="graph.subscribe.node" />}>
            {getFieldDecorator('nid', {
              rules: [{ required: true }],
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
            )}
          </FormItem>
          <FormItem label={<FormattedMessage id="graph.subscribe.screen" />}>
            {getFieldDecorator('scrrenId', {
              rules: [{ required: true }],
            })(
              <Select
                onDropdownVisibleChange={(dropdownVisible) => {
                  if (dropdownVisible) {
                    this.fetchScreenData();
                  }
                }}
              >
                {
                  _.map(this.state.screenData, (item) => {
                    return <Option key={item.id} value={item.id}>{item.name}</Option>;
                  })
                }
              </Select>,
            )}
          </FormItem>
          <FormItem label={<FormattedMessage id="graph.subscribe.tag" />}>
            {getFieldDecorator('subclassId', {
              rules: [{ required: true }],
            })(
              <Select
                onDropdownVisibleChange={(dropdownVisible) => {
                  if (dropdownVisible) {
                    this.fetchSubclassData();
                  }
                }}
              >
                {
                  _.map(this.state.subclassData, (item) => {
                    return <Option key={item.id} value={item.id}>{item.name}</Option>;
                  })
                }
              </Select>,
            )}
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(injectIntl(SubscribeModal)));
