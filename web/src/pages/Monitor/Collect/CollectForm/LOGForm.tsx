import React, { Component } from 'react';
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

class CollectForm extends Component<Props, State> {
  constructor(props: Props) {
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
      message.error('tags 上限三个');
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
      message.error('匹配正则不能为空');
    } else if (log === '') {
      message.error('log不能为空');
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
        message.error('动态日志 ${}中不能包含/');
        return;
      }
      // tags 数据转换成接口需要的格式，以及验证是否包含括号
      const bracketsReg = /\([^(]+\)/;
      const reservedKws = ['host', 'trigger', 'include'];
      if (tags.length) {
        const TagValidateStatus = _.every(tags, (o) => {
          if (o.name === '' || o.value === '') {
            message.error('tagName、tagValue 值不能为空');
            return false;
          }
          if (_.includes(reservedKws, o.name)) {
            message.error('tagName 不能包含 host、trigger、include 这些是odin系统保留关键字');
            return false;
          }
          if (!bracketsReg.test(o.value)) {
            message.error('tagValue 必须包含括号');
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
          message.error('添加采集配置的时候，请验证配置');
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
            label="归属节点"
          >
            {
              getFieldDecorator('nid', {
                initialValue: initialValues.nid,
                rules: [
                  { required: true, message: '不能为空' },
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
          <FormItem {...formItemLayout} label="监控指标名称">
            <Input
              addonBefore={params.action === 'add' || initialValues.name.indexOf('log.') === 0 ? 'log.' : null}
              {...getFieldProps('name', {
                initialValue: getPureName(initialValues.name),
                rules: [
                  {
                    required: true,
                    message: '不能为空',
                  },
                  nameRule,
                ],
              })}
              size="default"
              style={{ width: params.action === 'add' || initialValues.name.indexOf('log.') === 0 ? 500 : 500 }}
            />
          </FormItem>
          <FormItem {...formItemLayout} label="计算方法">
            <Select
              {...getFieldProps('func', {
                initialValue: initialValues.func,
                onChange: this.changeFunc.bind(this),
                rules: [
                  {
                    required: true,
                    message: '不能为空',
                  },
                ],
              })}
              size="default"
              style={{ width: 500 }}
            >
              <Option value="cnt">计数：对符合规则的日志进行计数</Option>
              <Option value="avg">平均：对符合规则的日志抓取出的数字进行平均</Option>
              <Option value="sum">求和：对符合规则的日志抓取出的数字进行求和</Option>
              <Option value="max">最大值：对符合规则的日志抓取出的数字取最大值</Option>
              <Option value="min">最小值：对符合规则的日志抓取出的数字进最小值</Option>
            </Select>
          </FormItem>
          <FormItem {...formItemLayout} label="日志路径">
            <Input
              {...getFieldProps('file_path', {
                initialValue: initialValues.file_path,
                rules: [
                  {
                    required: true,
                    message: '不能为空',
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
                    日志末尾自带时间格式，例如 {'/path/access.log.${%Y%m%d%H}'}<br />
                    {'${}中不能包含/'}
                  </div>
                }
              >
                <span>动态日志 <Icon type="info-circle-o" /></span>
              </Tooltip>
            </span>
          </FormItem>
          <FormItem {...formItemLayout} label="时间格式">
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
                      message: '不能为空',
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
              </Select>
            </div>
            <div style={{ marginLeft: 510, lineHeight: '20px' }}>
              时间格式必须和日志中的格式一样, 否则无法采集到数据。<br />
              如日志中出现多段符合时间正则的, 只使用第一个匹配结果。
            </div>
          </FormItem>
          <FormItem {...formItemLayout} label="采集周期">
            <Select
              size="default"
              style={{ width: 100 }}
              {...getFieldProps('step', {
                initialValue: initialValues.step,
                rules: [
                  { required: true, message: '不能为空' },
                ],
              })}
            >
              {
                _.map(interval, item => <Option key={item} value={item}>{item}</Option>)
              }
            </Select> 秒
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={
              <Tooltip
                title={
                  <div>
                    请填写正则表达式<br />
                    如计算方式选择了耗时: 必须包含括号( )<br />
                    例如 cost=(\d+) , 则取\d+的部分（默认以第一个括号为准）
                  </div>
                }
              >
                <span>匹配正则 <Icon type="info-circle-o" /></span>
              </Tooltip>
            }
          >
            <Input
              {...getFieldProps('pattern', {
                initialValue: initialValues.pattern,
                rules: [
                  {
                    required: true,
                    message: '不能为空',
                  },
                ],
              })}
              size="default"
              style={{ width: 500 }}
              placeholder="耗时计算：正则( )中的数值会用于计算曲线值；流量计数：每匹配到该正则，曲线值+1"
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
                          placeholder="不是曲线值! 匹配结果必须可枚举!"
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
                <Icon type="plus" />新增tag
              </Button>
            </div>
            <div style={{ marginLeft: 510, lineHeight: '20px' }}>
              <h4>tagName填写说明</h4>
              <div>1. 不允许使用host、trigger、include</div>
              <div>2. 不允许包含如下4个特殊字符= , : @</div>
              <h4>tagValue填写说明</h4>
              <div>1. 必须包含<span style={{ color: '#B03A5B' }}>括号</span>。括号中的正则内容被用作tagValue的取值，必须可枚举。</div>
              <div>2. 不允许包含如下4个特殊字符= , : @</div>
            </div>
          </FormItem>
          <FormItem {...formItemLayout} label="配置验证" required={this.state.logCheckVisible}>
            {
              this.state.logCheckVisible ?
                <div>
                  <Input
                    type="textarea"
                    placeholder="01/Jan/2006:15:04:05 输入一段日志内容验证配置..."
                    style={{ width: 500 }}
                    value={this.state.log}
                    onChange={(e) => {
                      this.setState({
                        log: e.target.value,
                      });
                    }}
                  />
                  <span style={{ paddingLeft: 10 }}>
                    请输入一行待监控的完整日志，包括时间。
                    <Tooltip title={
                      <div style={{ wordBreak: 'break-all', wordWrap: 'break-word' }}>
                        正确匹配：<br />输出正则匹配结果完整式及子项，输出tag正则匹配结果完整式及子项，以及时间匹配结果
                        <br />错误匹配：
                        <br />输出错误信息
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
                      验证
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
                  我的配置是否有问题?
                </Button>
            }
          </FormItem>
          <FormItem {...formItemLayout} label="备注">
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
            <Button type="primary" htmlType="submit" loading={this.state.submitLoading}>提交</Button>
            <Button
              style={{ marginLeft: 8 }}
            >
              <Link to={{ pathname: '/monitor/collect' }}>返回</Link>
            </Button>
          </FormItem>
        </Form>
        <Modal
          title={
            <span>
              验证结果：
              {
                this.state.logCheckedResultsSuccess ?
                  <span style={{ color: '#87d068' }}>成功</span> :
                  <span style={{ color: '#f50' }}>失败</span>
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
              关闭
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

export default Form.create()(CollectForm);
