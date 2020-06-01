import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Link } from 'react-router-dom';
import _ from 'lodash';
import { Button, Form, Select, Input, Modal, message, Icon, Tooltip, Row, Col, TreeSelect } from 'antd';
import { FormProps } from 'antd/lib/form';
import { renderTreeNodes } from '@cpts/Layout/utils';
import request from '@common/request';
import api from '@common/api';
import { nameRule, interval } from '../config';

interface Props extends FormProps {
  params: any,
  initialValues: any,
  treeData: any[],
  onSubmit: (values: any) => Promise<any>,
}

interface State {
  submitLoading: boolean,
  log: string,
  logChecked: boolean,
  logCheckVisible: boolean,
  logCheckLoading: boolean,
  logCheckedResultsVisible: boolean,
  logCheckedResultsSuccess: boolean,
  logCheckedResults: any[],
}

const FormItem = Form.Item;
const { Option } = Select;
const formItemLayout = {
  labelCol: { span: 6 },
  wrapperCol: { span: 18 },
};
const defaultFormData = {
  collect_type: 'log',
  func: 'cnt',
  func_type: 'FLOW',
  unit: '次数',
  time_format: 'dd/mmm/yyyy:HH:MM:SS',
  step: 10,
};
function getPureName(name: string) {
  if (name.indexOf('log.') === 0) {
    return _.split(name, 'log.')[1];
  }
  return name;
}

class CollectForm extends Component<Props & WrappedComponentProps, State> {
  constructor(props: Props & WrappedComponentProps) {
    super(props);
    const { params } = this.props;
    this.state = {
      submitLoading: false,
      log: '',
      logChecked: false,
      logCheckVisible: params.action === 'add',
      logCheckLoading: false,
      logCheckedResultsVisible: false,
      logCheckedResultsSuccess: false,
      logCheckedResults: [],
    } as State;
  }

  getInitialValues() {
    const data = _.assignIn({}, defaultFormData, _.cloneDeep(this.props.initialValues));
    data.name = data.name || '';
    data.tags = _.map(data.tags, (val, key) => {
      return {
        name: key,
        value: val,
      };
    });
    return data;
  }

  changeTag = (e: any, index: string, keyname: string) => {
    const { form } = this.props;
    const { value } = e.target;
    const tags = form!.getFieldValue('tags');

    tags[index][keyname] = value;
    form!.setFieldsValue({
      tags,
    });
  }

  addTag = () => {
    const { form } = this.props;
    const tags = form!.getFieldValue('tags');
    if (tags.length < 3) {
      tags.push({
        name: '',
        value: '',
      });
      form!.setFieldsValue({
        tags,
      });
    } else {
      message.error(this.props.intl.formatMessage({ id: 'collect.log.msg.tag.maximum' }));
    }
  }

  deleteTag = (index: string) => {
    const { form } = this.props;
    const tags = form!.getFieldValue('tags');
    tags.splice(index, 1);
    form!.setFieldsValue({
      tags,
    });
  }

  changeFunc = (val: string) => {
    const { form } = this.props;
    let funcType;
    if (val === 'cnt') {
      funcType = 'FLOW';
    } else {
      funcType = 'COSTTIME';
    }
    form!.setFieldsValue({
      func_type: funcType,
    });
  }

  checkLog = () => {
    const { log } = this.state;
    const { form } = this.props;
    const pattern = form!.getFieldValue('pattern');
    const timeFormat = form!.getFieldValue('time_format');
    const tagsReg: { [index: string]: string } = {};

    _.each(form!.getFieldValue('tags'), ({ name, value }) => {
      tagsReg[name] = value;
    });

    if (pattern === '') {
      message.error(this.props.intl.formatMessage({ id: 'collect.log.msg.pattern.empty' }));
    } else if (log === '') {
      message.error(this.props.intl.formatMessage({ id: 'collect.log.msg.log.empty' }));
    } else {
      this.setState({ logChecked: true, logCheckLoading: true });
      request(`${api.collect}/check`, {
        method: 'POST',
        body: JSON.stringify({
          ...tagsReg,
          re: pattern,
          log,
          time: timeFormat,
        }),
      }).then((res) => {
        this.setState({
          logCheckedResultsVisible: true,
          logCheckedResultsSuccess: res.success,
          logCheckedResults: res.tags || [],
        });
      }).finally(() => {
        this.setState({ logCheckLoading: false });
      });
    }
  }

  closeLogCheckedResults = () => {
    this.setState({
      logCheckedResultsVisible: false,
    });
  }

  handleSubmit = (e: any) => {
    e.preventDefault();
    const { onSubmit } = this.props;
    const initialValues = this.getInitialValues();
    this.props.form!.validateFields((errors, values) => {
      if (errors) {
        console.error(errors);
        return;
      }
      const { file_path: filePath, tags } = values;
      // 动态日志 验证是否包含 /
      const dynamicLogReg = /\$\{[^{]+\}/;
      const dynamicLogRegMatch = filePath.match(dynamicLogReg);
      if (dynamicLogRegMatch && dynamicLogRegMatch.length && _.some(dynamicLogRegMatch, n => _.includes(n, '/'))) {
        message.error('/ cannot be included in ${}');
        return;
      }
      // tags 数据转换成接口需要的格式，以及验证是否包含括号
      const bracketsReg = /\([^(]+\)/;
      const reservedKws = ['host', 'trigger', 'include'];
      if (tags.length) {
        const TagValidateStatus = _.every(tags, (o) => {
          if (o.name === '' || o.value === '') {
            message.error('tagName or tagValue is required');
            return false;
          }
          if (_.includes(reservedKws, o.name)) {
            message.error('Can not include the host trigger include these are the reserved keywords for the Odin');
            return false;
          }
          if (!bracketsReg.test(o.value)) {
            message.error('tagValue must include parentheses');
            return false;
          }
          return true;
        });
        if (!TagValidateStatus) {
          return;
        }
        values.tags = {};
        _.each(tags, ({ name, value }) => {
          values.tags[name] = value;
        });
      } else {
        delete values.tags;
      }
      // 添加采集配置的时候，需要做配置验证
      const { params = {} } = this.props;
      if (params.action === 'add') {
        if (!this.state.logChecked) {
          message.error('Verify the configuration when adding the collection configuration');
          return;
        }
      }
      // 新增、以及修改以 LOG. 开头的 name 做补全 LOG. 处理
      if (params.action === 'add' || initialValues.name.indexOf('log.') === 0) {
        values.name = `log.${values.name}`;
      }

      this.setState({
        submitLoading: true,
      });

      onSubmit(values).catch(() => {
        this.setState({
          submitLoading: false,
        });
      });
    });
  }

  render() {
    const { form, params } = this.props;
    const initialValues = this.getInitialValues();
    const { getFieldProps, getFieldValue, getFieldDecorator } = form!;
    getFieldProps('collect_type', {
      initialValue: initialValues.collect_type,
    });
    getFieldProps('func_type', {
      initialValue: initialValues.func_type,
    });
    getFieldProps('tags', {
      initialValue: initialValues.tags,
    });
    const tags = getFieldValue('tags');
    return (
      <div>
        <Form layout="horizontal" onSubmit={this.handleSubmit}>
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
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.name" />}>
            <Input
              addonBefore={params.action === 'add' || initialValues.name.indexOf('log.') === 0 ? 'log.' : null}
              {...getFieldProps('name', {
                initialValue: getPureName(initialValues.name),
                rules: [
                  { required: true },
                  nameRule,
                ],
              })}
              size="default"
              style={{ width: params.action === 'add' || initialValues.name.indexOf('log.') === 0 ? 500 : 500 }}
            />
          </FormItem>
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.func" />}>
            <Select
              {...getFieldProps('func', {
                initialValue: initialValues.func,
                onChange: this.changeFunc.bind(this),
                rules: [
                  {
                    required: true,
                  },
                ],
              })}
              size="default"
              style={{ width: 500 }}
            >
              <Option value="cnt"><FormattedMessage id="collect.log.func.cnt" /></Option>
              <Option value="avg"><FormattedMessage id="collect.log.func.avg" /></Option>
              <Option value="sum"><FormattedMessage id="collect.log.func.sum" /></Option>
              <Option value="max"><FormattedMessage id="collect.log.func.max" /></Option>
              <Option value="min"><FormattedMessage id="collect.log.func.min" /></Option>
            </Select>
          </FormItem>
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.path" />}>
            <Input
              {...getFieldProps('file_path', {
                initialValue: initialValues.file_path,
                rules: [
                  {
                    required: true,
                  },
                ],
              })}
              size="default"
              style={{ width: 500 }}
            />
            <span style={{ paddingLeft: 10 }}>
              <Tooltip
                overlayClassName="largeTooltip"
                title={
                  <div style={{ wordBreak: 'break-all', wordWrap: 'break-word' }}>
                    <FormattedMessage id="collect.log.path.dynamic.tip.1" /> {'/path/access.log.${%Y%m%d%H}'}<br />
                    <FormattedMessage id="collect.log.path.dynamic.tip.2" />
                  </div>
                }
              >
                <span><FormattedMessage id="collect.log.path.dynamic" /> <Icon type="info-circle-o" /></span>
              </Tooltip>
            </span>
          </FormItem>
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.timeFmt" />}>
            <div
              style={{
                width: 500, float: 'left', position: 'relative', zIndex: 1,
              }}
            >
              <Select
                {...getFieldProps('time_format', {
                  initialValue: initialValues.time_format,
                  rules: [
                    {
                      required: true,
                    },
                  ],
                })}
                size="default"
                style={{ width: 500 }}
              >
                <Option value="dd/mmm/yyyy:HH:MM:SS">01/Jan/2006:15:04:05</Option>
                <Option value="dd/mmm/yyyy HH:MM:SS">01/Jan/2006 15:04:05</Option>
                <Option value="yyyy-mm-ddTHH:MM:SS">2006-01-02T15:04:05</Option>
                <Option value="dd-mmm-yyyy HH:MM:SS">01-Jan-2006 15:04:05</Option>
                <Option value="yyyy-mm-dd HH:MM:SS">2006-01-02 15:04:05</Option>
                <Option value="yyyy/mm/dd HH:MM:SS">2006/01/02 15:04:05</Option>
                <Option value="yyyymmdd HH:MM:SS">20060102 15:04:05</Option>
                <Option value="mmm dd HH:MM:SS">Jan 2 15:04:05</Option>
                <Option value="mmdd HH:MM:SS">0102 15:04:05</Option>
                <Option value="dd/mm/yyyy:HH:MM:SS">02/01/2006:15:04:05</Option>
              </Select>
            </div>
            <div style={{ marginLeft: 510, lineHeight: '20px' }}>
              <FormattedMessage id="collect.log.timeFmt.help.1" /><br />
              <FormattedMessage id="collect.log.timeFmt.help.2" />
            </div>
          </FormItem>
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.step" />}>
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
            </Select> <FormattedMessage id="collect.log.step.unit" />
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={
              <Tooltip
                title={
                  <div>
                    <FormattedMessage id="collect.log.pattern.tip.1" /><br />
                    <FormattedMessage id="collect.log.pattern.tip.2" /><br />
                    <FormattedMessage id="collect.log.pattern.tip.3" />
                  </div>
                }
              >
                <span><FormattedMessage id="collect.log.pattern" /> <Icon type="info-circle-o" /></span>
              </Tooltip>
            }
          >
            <Input
              {...getFieldProps('pattern', {
                initialValue: initialValues.pattern,
                rules: [
                  {
                    required: true,
                  },
                ],
              })}
              size="default"
              style={{ width: 500 }}
            />
          </FormItem>
          <FormItem {...formItemLayout} label="tags">
            <div
              style={{
                width: 500, float: 'left', position: 'relative', zIndex: 1,
              }}
            >
              {
                _.map(tags, ({ name, value }, index) => {
                  return (
                    <Row gutter={16} key={index}>
                      <Col span={8}>
                        <Input
                          addonBefore="tagName"
                          value={name}
                          onChange={(e) => {
                            this.changeTag(e, index, 'name');
                          }}
                        />
                      </Col>
                      <Col span={13}>
                        <Input
                          addonBefore="tagValue"
                          placeholder={this.props.intl.formatMessage({ id: 'collect.log.tagval.placeholder' })}
                          value={value}
                          onChange={(e) => {
                            this.changeTag(e, index, 'value');
                          }}
                        />
                      </Col>
                      <Col span={1}>
                        <Button
                          size="default"
                          onClick={() => { this.deleteTag(index); }}
                        >
                          <Icon type="close" />
                        </Button>
                      </Col>
                    </Row>
                  );
                })
              }
              <Button
                size="default"
                onClick={this.addTag}
              >
                <Icon type="plus" /><FormattedMessage id="collect.log.tags.add" />
              </Button>
            </div>
            <div style={{ marginLeft: 510, lineHeight: '20px' }}>
              <h4><FormattedMessage id="collect.log.tagName.help.title" /></h4>
              <div><FormattedMessage id="collect.log.tagName.help.1" /></div>
              <div><FormattedMessage id="collect.log.tagName.help.2" /></div>
              <h4><FormattedMessage id="collect.log.tagValue.help.title" /></h4>
              <div><FormattedMessage id="collect.log.tagValue.help.1" /></div>
              <div><FormattedMessage id="collect.log.tagValue.help.2" /></div>
            </div>
          </FormItem>
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.check" />} required={this.state.logCheckVisible}>
            {
              this.state.logCheckVisible ?
                <div>
                  <Input
                    type="textarea"
                    placeholder="01/Jan/2006:15:04:05"
                    style={{ width: 500 }}
                    value={this.state.log}
                    onChange={(e) => {
                      this.setState({
                        log: e.target.value,
                      });
                    }}
                  />
                  <span style={{ paddingLeft: 10 }}>
                    <FormattedMessage id="collect.log.check.help" />
                    <Tooltip title={
                      <div style={{ wordBreak: 'break-all', wordWrap: 'break-word' }}>
                        <FormattedMessage id="collect.log.check.help.tip.1" /><br /><FormattedMessage id="collect.log.check.help.tip.2" />
                        <br /><FormattedMessage id="collect.log.check.help.tip.3" />
                        <br /><FormattedMessage id="collect.log.check.help.tip.4" />
                      </div>
                    }
                    >
                      <span><Icon type="info-circle-o" /></span>
                    </Tooltip>
                  </span>
                  <div>
                    <Button
                      size="default"
                      onClick={this.checkLog}
                      loading={this.state.logCheckLoading}
                    >
                      <FormattedMessage id="collect.log.check.btn" />
                    </Button>
                  </div>
                </div> :
                <Button
                  size="default"
                  onClick={() => {
                    this.setState({
                      // eslint-disable-next-line react/no-access-state-in-setstate
                      logCheckVisible: !this.state.logCheckVisible,
                    });
                  }}
                >
                  <FormattedMessage id="collect.log.check.btn2" />
                </Button>
            }
          </FormItem>
          <FormItem {...formItemLayout} label={<FormattedMessage id="collect.log.note" />}>
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
        <Modal
          title={
            <span>
              Result：
              {
                this.state.logCheckedResultsSuccess ?
                  <span style={{ color: '#87d068' }}>success</span> :
                  <span style={{ color: '#f50' }}>error</span>
              }
            </span>
          }
          visible={this.state.logCheckedResultsVisible}
          onOk={this.closeLogCheckedResults}
          onCancel={this.closeLogCheckedResults}
          footer={[
            <Button
              key="back"
              type="primary"
              size="large"
              onClick={this.closeLogCheckedResults}
            >
              close
            </Button>,
          ]}
        >
          <div>
            <Form layout="horizontal">
              {
                // eslint-disable-next-line consistent-return
                _.map(this.state.logCheckedResults, (result, i) => {
                  for (const keyName in result) {
                    return (
                      <FormItem
                        key={i}
                        labelCol={{ span: 4 }}
                        wrapperCol={{ span: 19 }}
                        label={keyName}
                      >
                        <Input disabled type="textarea" value={result[keyName]} />
                      </FormItem>
                    );
                  }
                })
              }
            </Form>
          </div>
        </Modal>
      </div>
    );
  }
}

export default Form.create()(injectIntl(CollectForm));
