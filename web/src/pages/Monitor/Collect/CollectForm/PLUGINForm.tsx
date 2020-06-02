import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import _ from 'lodash';
import { Button, Form, Select, Input, TreeSelect, Icon, Row, Col} from 'antd';
import { useDynamicList } from '@umijs/hooks'
import { injectIntl, FormattedMessage } from 'react-intl';
import { renderTreeNodes } from '@cpts/Layout/utils';
import AceEditor from '@cpts/AceEditor';
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

  if (initialValues.env) {
    try {
      const env = JSON.parse(initialValues.env);
      initialValues.env = _.map(env, (value, name) => {
        return {
          name, value,
        };
      });
    } catch (e) {
      console.log(e);
    }
  }

  const { list, remove, getKey, push, resetList } = useDynamicList(initialValues.env);


  useEffect(() => {
    resetList(initialValues.env);
  }, [JSON.stringify(initialValues.env)]);

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
      if (values.env) {
        const { env } = values;
        const newEnv: any = {};
        _.forEach(env, (item) => {
          newEnv[item.name] = item.value;
        });
        values.env = JSON.stringify(newEnv);
      }
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
      <FormItem {...formItemLayout} label={<FormattedMessage id="collect.plugin.env"/>}>
        {
          _.map(list, (item: any, index: number) => {
            return (
              <Row key={getKey(index)} gutter={10}>
                <Col span={9}>
                  <FormItem>
                    {getFieldDecorator(`env[${getKey(index)}].name`, { initialValue: item.name, rules: [{ required: true }] })(
                      <Input placeholder="field name" style={{ width: '100%' }} />,
                    )}
                  </FormItem>
                </Col>
                <Col span={10}>
                  <FormItem>
                    {getFieldDecorator(`env[${getKey(index)}].value`, { initialValue: item.value, rules: [{ required: true }] })(
                      <Input placeholder="field value" style={{ width: '100%' }} />,
                    )}
                  </FormItem>
                </Col>
                <Col span={5}>
                  {list.length > 0 && (
                    <Icon
                      type="minus-circle-o"
                      style={{ marginLeft: 8 }}
                      onClick={() => {
                        remove(index);
                      }}
                    />
                  )}
                </Col>
              </Row>
            );
          })
        }
        <Icon
          type="plus-circle-o"
          style={{ marginLeft: 8 }}
          onClick={() => {
            push({ name: '', value: '' });
          }}
        />
      </FormItem>
      <FormItem {...formItemLayout} label="Stdin">
        <AceEditor
          placeholder=""
          {...getFieldProps('stdin', {
            initialValue: initialValues.stdin,
          })}
          style={{ width: 500, height: 200 }}
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
          <Link to={{ pathname: '/monitor/collect' }}>{<FormattedMessage id="form.goback" />}</Link>
        </Button>
      </FormItem>
    </Form>
  );
}

export default Form.create()(injectIntl(CollectForm));
