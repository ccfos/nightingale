import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import { Button, Form, Input, DatePicker } from 'antd';
import { FormProps } from 'antd/lib/form';
import moment from 'moment';
import _ from 'lodash';

interface Props extends FormProps {
  category: string,
  readOnly: boolean,
  initialValues: any,
}

const ButtonGroup = Button.Group;
const FormItem = Form.Item;
const { TextArea } = Input;
const formItemLayout = {
  labelCol: { span: 6 },
  wrapperCol: { span: 14 },
};
const timeFormatMap = {
  antd: 'YYYY-MM-DD HH:mm:ss',
  moment: 'YYYY-MM-DD HH:mm:ss',
};
const shortcutBar = [
  { label: '1小时', value: 3600 },
  { label: '2小时', value: 7200 },
  { label: '6小时', value: 21600 },
  { label: '12小时', value: 43200 },
  { label: '1天', value: 86400 },
  { label: '2天', value: 172800 },
  { label: '7天', value: 604800 },
];

class CustomForm extends Component<Props> {
  static defaultProps = {
    readOnly: false,
    initialValues: {},
  };

  state = {};

  checkTags(rule: any, value: any, callback: any) {
    if (value) {
      const currentTag = _.get(value, '[0]', {});
      if (!currentTag.tkey || _.isEmpty(currentTag.tval)) {
        callback('tag名称和取值不能为空');
      } else {
        callback();
      }
    } else {
      callback();
    }
  }

  updateSilenceTime(val: number) {
    // eslint-disable-next-line react/prop-types
    const { setFieldsValue } = this.props.form!;
    const now = moment();
    const beginTs = now.clone();
    const endTs = now.clone().add(val, 'seconds');

    setFieldsValue({ btime: beginTs });
    setFieldsValue({ etime: endTs });
  }

  renderTimeOptions() {
    const { readOnly } = this.props;
    const { getFieldValue } = this.props.form!;
    const beginTs = getFieldValue('btime');
    const endTs = getFieldValue('etime');
    let timeSpan: number;

    if (beginTs && endTs) {
      timeSpan = endTs.unix() - beginTs.unix();
    }

    if (readOnly) {
      return null;
    }
    return (
      <ButtonGroup
        size="default"
      >
        {
          _.map(shortcutBar, o => (
            <Button
              onClick={() => { this.updateSilenceTime(o.value); }}
              key={o.value}
              type={o.value === timeSpan ? 'primary' : undefined}
            >
              <FormattedMessage id={o.label} />
            </Button>
          ))
        }
      </ButtonGroup>
    );
  }

  render() {
    const { readOnly, initialValues } = this.props;
    const { getFieldDecorator } = this.props.form!;

    return (
      <div className="alarm-shielding-form">
        <Form className={readOnly ? 'readOnly' : ''}>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="silence.form.metric" />}
          >
            {getFieldDecorator('metric', {
              initialValue: initialValues.metric,
              rules: [
                { required: false },
              ],
            })(
              <Input />,
            )}
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="silence.form.endpoints" />}
          >
            {getFieldDecorator('endpoints', {
              initialValue: _.isArray(initialValues.endpoints) ? _.join(initialValues.endpoints, '\n') : initialValues.endpoints,
              rules: [
                { required: true },
              ],
            })(
              <TextArea
                autosize={{ minRows: 2, maxRows: 6 }}
                disabled={readOnly}
              />,
            )}
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="silence.form.tags" />}
            help="eg. key1=value1,key2=value2"
          >
            {getFieldDecorator('tags', {
              initialValue: initialValues.tags,
            })(
              <TextArea
                autosize={{ minRows: 2, maxRows: 6 }}
                disabled={readOnly}
              />,
            )}
          </FormItem>
          <FormItem
            wrapperCol={{ span: 14, offset: 6 }}
          >
            {this.renderTimeOptions()}
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="silence.form.stime" />}
          >
            {getFieldDecorator('btime', {
              initialValue: moment.unix(initialValues.btime),
              rules: [
                { required: true },
              ],
            })(
              <DatePicker
                showTime
                format={timeFormatMap.antd}
                disabled={readOnly}
              />,
            )}
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="silence.form.etime" />}
          >
            {getFieldDecorator('etime', {
              initialValue: moment.unix(initialValues.etime),
              rules: [
                { required: true },
              ],
            })(
              <DatePicker
                showTime
                format={timeFormatMap.antd}
                disabled={readOnly}
              />,
            )}
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="silence.cause" />}
          >
            {getFieldDecorator('cause', {
              initialValue: initialValues.cause,
              rules: [
                { required: true },
              ],
            })(
              <TextArea
                autosize={{ minRows: 2, maxRows: 6 }}
                disabled={readOnly}
              />,
            )}
          </FormItem>
        </Form>
      </div>
    );
  }
}

export default Form.create()(CustomForm);
