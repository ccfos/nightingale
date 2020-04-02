import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import { Modal, Form, TreeSelect } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { TreeNode } from '@interface';
import { normalizeTreeData, renderTreeNodes } from '@cpts/Layout/utils';

interface Props {
  treeNodes: TreeNode[],
  title: string,
  visible: boolean,
  onOk: (nid: number) => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;

class BatchCloneToNidModal extends Component<Props & FormProps> {

  static defaultProps = {
    treeNodes: [],
    title: '',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  state = {
    treeData: [],
  }

  componentDidMount = () => {
    const treeData = normalizeTreeData(_.cloneDeep(this.props.treeNodes));
    this.setState({ treeData });
  }

  handleOk = () => {
    this.props.form!.validateFields(async (err, values) => {
      if (!err) {
        this.props.onOk(values.nid);
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
      >
        <Form layout="vertical">
          <FormItem
            label={<FormattedMessage id="collect.common.node" />}
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
          </FormItem>
        </Form>
      </Modal>
    );
  }
}

export default ModalControl(Form.create()(BatchCloneToNidModal));
