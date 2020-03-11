import React, { Component } from 'react';
import { Modal, Form, TreeSelect, Select } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import ModalControl from '@cpts/ModalControl';
import { normalizeTreeData, renderTreeNodes } from '@cpts/Layout/utils';
import request from '@common/request';
import api from '@common/api';

interface Props extends FormProps {
  data: any,
  treeData: any,
  title: string,
  visible: boolean,
  onOk: (values: any) => void,
  onCancel: () => void,
  destroy: () => void,
}

const FormItem = Form.Item;
const { Option } = Select;

class BatchMoveSubclass extends Component<Props> {
  static defaultProps = {
    title: '批量移动分类',
    visible: true,
    onOk: _.noop,
    onCancel: _.noop,
    destroy: _.noop,
  };

  state = {
    screenData: [],
  };

  handleOk = () => {
    this.props.form!.validateFields((err, values) => {
      if (!err) {
        this.props.onOk(values);
        this.props.destroy();
      }
    });
  }

  handleCancel = () => {
    this.props.destroy();
  }

  handleSelectedTreeNodeIdChange = (nid: number) => {
    request(`${api.node}/${nid}/screen`).then((res) => {
      this.setState({ screenData: res || [] });
    });
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
        <Form layout="vertical" onSubmit={(e) => {
          e.preventDefault();
          this.handleOk();
        }}>
          <FormItem label="需要移动的分类">
            {getFieldDecorator('subclasses', {
              rules: [{ required: true, message: '请选择分类!' }],
            })(
              <Select mode="multiple">
                {
                  _.map(this.props.data, (item) => {
                    return <Option key={item.id} value={item.id}>{item.name}</Option>;
                  })
                }
              </Select>,
            )}
          </FormItem>
          <FormItem label="将要移动到的节点">
            {getFieldDecorator('nid', {
              rules: [{ required: true, message: '请选择节点!' }],
              onChange: this.handleSelectedTreeNodeIdChange,
            })(
              <TreeSelect
                showSearch
                allowClear
                treeNodeFilterProp="title"
                treeNodeLabelProp="path"
                dropdownStyle={{ maxHeight: 200, overflow: 'auto' }}
              >
                {renderTreeNodes(normalizeTreeData(this.props.treeData))}
              </TreeSelect>,
            )}
          </FormItem>
          <FormItem label="将要移动到的大盘">
            {getFieldDecorator('screenId', {
              rules: [{ required: true, message: '请选择大盘!' }],
            })(
              <Select>
                {
                  _.map(this.state.screenData, (item: any) => {
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

export default ModalControl(Form.create()(BatchMoveSubclass));
