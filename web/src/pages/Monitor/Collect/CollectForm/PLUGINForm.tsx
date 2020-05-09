import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import _ from 'lodash';
import { Button, Form, Select, Input, TreeSelect } from 'antd';
import { injectIntl, FormattedMessage } from 'react-intl';
import { renderTreeNodes } from '@cpts/Layout/utils';
import { nameRule, interval } from '../config';


const FormItem = Form.Item;
const { Option } = Select;
const formItemLayout = {
  labelCol: { span: 6 },
  wrapperCol: { span: 14 },
};
const defaultFormData = {
  collect_type: 'plugin',
  timeout: 3,
  step: 10,
};

const getInitialValues = (initialValues: any) => {
  return _.assignIn({}, defaultFormData, _.cloneDeep(initialValues));
}

const CollectForm = (props: any) => {
  const initialValues = getInitialValues(props.initialValues);
  const { getFieldProps, getFieldDecorator } = props.form;

  getFieldDecorator('collect_type', {
    initialValue: initialValues.collect_type,
  });

  const [submitLoading, setSubmitLoading] = useState(false);

  const handleSubmit = (e: any) => {
    e.preventDefault();
    props.form.validateFields((errors: any, values: any) => {
      if (errors) {
        console.error(errors);
        return;
      }
      setSubmitLoading(true);
      props.onSubmit(values).catch(() => {
        setSubmitLoading(false);
      });
    });
  }

  return (
    <Form layout="horizontal" onSubmit={handleSubmit}>
      <FormItem
        {...formItemLayout}
        label={<FormattedMessage id="collect.common.node" />}
      >
        {
          getFieldDecorator('nid', {
            initialValue: initialValues.nid,
            rules: [{ required: true }],
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
              {renderTreeNodes(props.treeData)}
            </TreeSelect>,
          )
        }
      </FormItem>
      <FormItem {...formItemLayout} label={<FormattedMessage id="collect.common.name" />}>
        <Input
          {...getFieldProps('name', {
            initialValue: initialValues.name,
            rules: [
              { required: true },
              nameRule,
            ],
          })}
          size="default"
          style={{ width: 500 }}
          placeholder={props.intl.formatMessage({ id: 'collect.plugin.name.placeholder' })}
        />
      </FormItem>
      <FormItem {...formItemLayout} label={<FormattedMessage id="collect.plugin.filepath" />}>
        <Input
          {...getFieldProps('file_path', {
            initialValue: initialValues.file_path,
            rules: [{ required: true }]
          })}
          style={{ width: 500 }}
          placeholder={props.intl.formatMessage({ id: 'collect.plugin.filepath.placeholder' })}
        />
      </FormItem>
      <FormItem {...formItemLayout} label={<FormattedMessage id="collect.plugin.params" />}>
        <Input
          {...getFieldProps('params', {
            initialValue: initialValues.params,
          })}
          style={{ width: 500 }}
        />
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
        </Select> {<FormattedMessage id="collect.common.step.unit" />}
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
        <Button type="primary" htmlType="submit" loading={submitLoading}>{<FormattedMessage id="form.submit" />}</Button>
        <Button
          style={{ marginLeft: 8 }}
        >
          <Link to={{ pathname: '/collect' }}>{<FormattedMessage id="form.goback" />}</Link>
        </Button>
      </FormItem>
    </Form>
  );
}

export default Form.create()(injectIntl(CollectForm));
