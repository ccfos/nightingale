import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Link } from 'react-router-dom';
import _ from 'lodash';
import { Button, Form, Select, Input, InputNumber, TreeSelect } from 'antd';
import { FormProps } from 'antd/lib/form';
import { renderTreeNodes } from '@cpts/Layout/utils';
import { nameRule, interval } from '../config';

interface Props extends FormProps {
  params: any,
  initialValues: any,
  treeData: any[],
  onSubmit: (values: any) => Promise<any>,
}

const FormItem = Form.Item;
const { Option } = Select;
const formItemLayout = {
  labelCol: { span: 6 },
  wrapperCol: { span: 14 },
};
const defaultFormData = {
  collect_type: 'port',
  timeout: 3,
  step: 10,
};

class CollectForm extends Component<Props & WrappedComponentProps> {
  state = {
    submitLoading: false,
  };

  getInitialValues() {
    const data = _.assignIn({}, defaultFormData, _.cloneDeep(this.props.initialValues));
    return data;
  }

  handleSubmit = (e: any) => {
    e.preventDefault();
    const { onSubmit } = this.props;
    this.props.form!.validateFields((errors, values) => {
      if (errors) {
        console.error(errors);
        return;
      }
      this.setState({
        submitLoading: true,
      });
      const { service } = values;
      values.tags = `service=${service}`;
      delete values.service;
      onSubmit(values).catch(() => {
        this.setState({
          submitLoading: false,
        });
      });
    });
  }

  render() {
    const { form } = this.props;
    const initialValues = this.getInitialValues();
    const { getFieldDecorator, getFieldProps } = form! as any;
    const service = _.chain(initialValues.tags).split(',').filter(item => item.indexOf('service=') === 0).head().split('service=').last().value();
    getFieldProps('collect_type', {
      initialValue: initialValues.collect_type,
    });
    return (
      <Form layout="horizontal" onSubmit={this.handleSubmit}>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="collect.port.title" />}
        >
          <span className="ant-form-text">proc.port.listen</span>
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="collect.common.node" />}
        >
          {
            getFieldDecorator('nid', {
              initialValue: initialValues.nid,
              rules: [
                { required: true },
              ],
            })(
              <TreeSelect
                style={{ width: 500 }}
                showSearch
                allowClear
                treeDefaultExpandAll
                treeNodeFilterProp="title"
                treeNodeLabelProp="path"
                dropdownStyle={{ maxHeight: 400, overflow: 'auto' }}
              >
                {renderTreeNodes(this.props.treeData)}
              </TreeSelect>,
            )
          }
        </FormItem>
        <FormItem {...formItemLayout} label={<FormattedMessage id="collect.common.name" />}>
          <Input
            {...getFieldProps('name', {
              initialValue: initialValues.name,
              rules: [
                {
                  required: true,
                },
                nameRule,
              ],
            })}
            size="default"
            style={{ width: 500 }}
            placeholder={this.props.intl.formatMessage({ id: 'collect.port.name.placeholder' })}
          />
        </FormItem>
        <FormItem {...formItemLayout} label="service">
          <Input
            {...getFieldProps('service', {
              initialValue: service,
              rules: [
                { required: true },
                { pattern: /^[a-zA-Z0-9-_.]+$/, message: this.props.intl.formatMessage({ id: 'collect.port.pattern.msg' }) },
              ],
            })}
            size="default"
            style={{ width: 500 }}
            // placeholder="全局唯一的进程英文名"
          />
        </FormItem>
        <FormItem {...formItemLayout} label={<FormattedMessage id="collect.port.port" />} required>
          <InputNumber
            {...getFieldProps('port', {
              initialValue: initialValues.port,
              rules: [
                { required: true },
              ],
            })}
            size="default"
            style={{ width: 500 }}
            // placeholder="请输入端口号"
          />
        </FormItem>
        <FormItem {...formItemLayout} label={<FormattedMessage id="collect.port.timeout" />}>
          <InputNumber
            min={1}
            style={{ width: 100 }}
            size="default"
            {...getFieldProps('timeout', {
              initialValue: initialValues.timeout,
              rules: [
                { required: true },
              ],
            })}
          /> <FormattedMessage id="collect.port.timeout.unit" />
        </FormItem>
        <FormItem {...formItemLayout} label={<FormattedMessage id="collect.common.step" />}>
          <Select
            size="default"
            style={{ width: 100 }}
            {...getFieldProps('step', {
              initialValue: initialValues.step,
              rules: [
                { required: true },
              ],
            })}
          >
            {
              _.map(interval, item => <Option key={item} value={item}>{item}</Option>)
            }
          </Select> <FormattedMessage id="collect.common.step.unit" />
        </FormItem>
        <FormItem {...formItemLayout} label={<FormattedMessage id="collect.common.note" />}>
          <Input
            type="textarea"
            placeholder=""
            {...getFieldProps('comment', {
              initialValue: initialValues.comment,
            })}
            style={{ width: 500 }}
          />
        </FormItem>
        <FormItem wrapperCol={{ offset: 6 }} style={{ marginTop: 24 }}>
          <Button type="primary" htmlType="submit" loading={this.state.submitLoading}><FormattedMessage id="form.submit" /></Button>
          <Button
            style={{ marginLeft: 8 }}
          >
            <Link to={{ pathname: '/monitor/collect' }}><FormattedMessage id="form.goback" /></Link>
          </Button>
        </FormItem>
      </Form>
    );
  }
}

export default Form.create()(injectIntl(CollectForm));
