import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import PropTypes from 'prop-types';
import { Form, Button, Input, Radio, Tooltip, Icon, InputNumber, TreeSelect, Checkbox, Row, Col } from 'antd';
import { FormProps } from 'antd/lib/form';
import _ from 'lodash';
import queryString from 'query-string';
import { normalizeTreeData, renderTreeNodes, filterTreeNodes } from '@cpts/Layout/utils';
import { services } from '@cpts/Graph';
import request from '@common/request';
import api from '@common/api';
import { Expressions, Filters, Actions, PeriodTime, AlarmUpgrade } from './SettingFields/';
import { prefixCls } from '../config';
import { processReqData } from './utils';

interface Props extends FormProps{
  initialValues: any,
  onSubmit: (values: any) => void,
}

const FormItem = Form.Item;
const RadioGroup = Radio.Group;

class SettingFields extends Component<Props & WrappedComponentProps> {
  static contextTypes = {
    habitsId: PropTypes.string,
  };

  static defaultProps = {
    initialValues: {},
  };

  currentMetric: undefined | string = undefined;
  state = {
    metrics: [],
    tags: {},
    treeData: [],
    originTreeData: [],
    excludeTreeData: [],
    notifyDataLoading: false,
    notifyGroupData: [],
    notifyUserData: [],
    advanced: false,
  };

  constructor(props: Props & WrappedComponentProps) {
    super(props);
    this.fetchNotifyData = _.debounce(this.fetchNotifyData, 500);
  }

  componentDidMount() {
    this.fetchTreeData();
    this.fetchMetrics.call(this);
    this.fetchTagkvs(this.props.initialValues.strategy_expressions);
    this.fetchNotifyData();
  }

  fetchTreeData() {
    request(api.tree).then((res) => {
      this.setState({ treeData: res });
      const treeData = normalizeTreeData(res);
      this.setState({ treeData, originTreeData: res }, () => {
        if (this.props.initialValues.nid) {
          this.handleNsChange(this.props.initialValues.nid);
        }
      });
    });
  }

  async fetchMetrics() {
    const { nid } = this.props.initialValues;
    let hosts = [];
    let metrics = [];
    try {
      hosts = await services.fetchEndPoints(nid, this.context.habitsId);
    } catch (e) {
      console.log(e);
    }
    try {
      metrics = await request(`${api.graphIndex}/metrics`, {
        method: 'POST',
        body: JSON.stringify({ endpoints: hosts }),
      }).then((res) => {
        return res.metrics;
      });
    } catch (e) {
      console.log(e);
    }
    this.setState({ metrics });
  }

  fetchTagkvs(strategyExpressionsValue: any) {
    if (!strategyExpressionsValue) return;
    // 历史原因只取第一个 expression.metric
    const firstExpression = strategyExpressionsValue[0] || {};
    const { metric = '' } = firstExpression;
    const { nid } = this.props.initialValues;

    if (nid && metric && this.currentMetric !== metric) {
      request(`${api.graphIndex}/tagkv`, {
        method: 'POST',
        body: JSON.stringify({
          nid: [nid],
          metric: [metric],
        }),
      }).then((data) => {
        const tagkvsraw = _.sortBy(data.length > 0 ? data[0].tagkv : [], 'tagk');
        const tagkvs: any = {};

        _.each(tagkvsraw, (v) => {
          if (v && v.tagk && v.tagv) {
            tagkvs[v.tagk] = _.sortBy(v.tagv);
          }
        });
        this.currentMetric = metric;
        this.setState({
          tags: tagkvs,
        });
      });
    }
  }

  async fetchNotifyData(params = {}, params2 = {}) {
    this.setState({ notifyDataLoading: true });
    try {
      const query1 = queryString.stringify({
        limit: 1000,
        ...params,
      });
      const query2 = queryString.stringify({
        limit: 1000,
        ...params2,
      });
      const teamData = await request(`${api.team}?${query1}`);
      const userData = await request(`${api.user}?${query2}`);
      this.setState({
        notifyGroupData: teamData.list,
        notifyUserData: userData.list,
      });
    } catch (e) {
      console.log(e);
    }
    this.setState({ notifyDataLoading: false });
  }

  handleSubmit = (e: any) => {
    e.preventDefault();
    this.props.form!.validateFields((errors, values) => {
      if (errors) {
        console.log('Errors in form!!!', errors);
        return;
      }
      this.props.onSubmit(processReqData(values));
    });
  }

  handleExpressionsChange = (val: string) => {
    this.fetchTagkvs(val);
  }

  handleNsChange = (value: any) => {
    const excludeTreeData = filterTreeNodes(this.state.treeData, value);
    const treeDataChildren = _.filter(this.state.originTreeData, (item: any) => {
      return item.pid === value && item.leaf === 1;
    });
    this.setState({ treeDataChildren, excludeTreeData });
  }

  render() {
    const { getFieldDecorator, getFieldValue, setFieldsValue } = this.props.form!;
    const formItemLayout = {
      labelCol: { span: 4 },
      wrapperCol: { span: 16 },
    };

    getFieldDecorator('category', {
      initialValue: 1,
    });

    return (
      <Form className={`${prefixCls}-strategy-form`} layout="horizontal" onSubmit={this.handleSubmit}>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.name" />}
        >
          {
            getFieldDecorator('name', {
              initialValue: this.props.initialValues.name,
              rules: [{
                required: true
              }],
            })(
              <Input />,
            )
          }
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.node" />}
        >
          {
            getFieldDecorator('nid', {
              initialValue: this.props.initialValues.nid,
              onChange: (value: any) => {
                this.handleNsChange(value);
                setFieldsValue({
                  exclude_nid: [],
                });
              },
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
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.node.exclude" />}
        >
          {
            getFieldDecorator('excl_nid', {
              initialValue: this.props.initialValues.excl_nid,
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
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={
            <Tooltip title={
              <div>
                <FormattedMessage id="stra.priority.1.tip" /><br />
                <FormattedMessage id="stra.priority.2.tip" /><br />
                <FormattedMessage id="stra.priority.3.tip" />
              </div>
            }>
              <span><FormattedMessage id="stra.priority" /> <Icon type="info-circle-o" /></span>
            </Tooltip>
          }
          required
        >
          {
            getFieldDecorator('priority', {
              initialValue: this.props.initialValues.priority || 3,
            })(
              <RadioGroup size="default">
                {
                  _.map({
                    1: {
                      alias: <FormattedMessage id="stra.priority.1" />,
                      color: 'red',
                    },
                    2: {
                      alias: <FormattedMessage id="stra.priority.2" />,
                      color: 'yellow',
                    },
                    3: {
                      alias: <FormattedMessage id="stra.priority.3" />,
                      color: 'blue',
                    },
                  }, (val, key) => {
                    return <Radio key={key} value={Number(key)}>{val.alias}</Radio>;
                  })
                }
              </RadioGroup>,
            )
          }
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.alertDur" />}
        >
          {
            getFieldDecorator('alert_dur', {
              initialValue: this.props.initialValues.alert_dur !== undefined ? this.props.initialValues.alert_dur : 180,
            })(
              <InputNumber min={0} />,
            )
          }
          <FormattedMessage id="stra.seconds" />
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.trigger" />}
          validateStatus="success" // 兼容
          help="" // 兼容
        >
          {
            getFieldDecorator('exprs', {
              initialValue: this.props.initialValues.exprs || [Expressions.defaultExpressionValue],
              onChange: this.handleExpressionsChange,
              rules: [{
                validator: Expressions.checkExpressions,
              }],
            })(
              <Expressions
                alertDuration={getFieldValue('alert_dur')}
                headerExtra={<div>headerExtra</div>}
                metrics={this.state.metrics}
              />,
            )
          }
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.tag" />}
        >
          {
            getFieldDecorator('tags', {
              initialValue: this.props.initialValues.tags || [],
            })(
              <Filters
                tags={this.state.tags}
              />,
            )
          }
        </FormItem>
        <FormItem
          {...formItemLayout}
          label={<FormattedMessage id="stra.action" />}
          validateStatus="success" // 兼容
          help="" // 兼容
        >
          {
            getFieldDecorator('action', {
              initialValue: this.props.initialValues.action || Actions.defaultValue,
              rules: [{
                validator: Actions.checkActions,
              }],
            })(
              <Actions
                loading={this.state.notifyDataLoading}
                notifyGroupData={this.state.notifyGroupData}
                notifyUserData={this.state.notifyUserData}
                // eslint-disable-next-line react/jsx-no-bind
                fetchNotifyData={this.fetchNotifyData.bind(this)}
              />,
            )
          }
        </FormItem>
        <Row style={{ marginBottom: 10 }}>
          <Col offset={4}>
            <a
              onClick={() => {
                this.setState({ advanced: !this.state.advanced });
              }}
            ><FormattedMessage id="stra.advanced" /> <Icon type={this.state.advanced ? 'up' : 'down'} />
            </a>
          </Col>
        </Row>
        <div style={{ display: this.state.advanced ? 'block' : 'none' }}>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="stra.recovery.dur" />}
          >
            {
              getFieldDecorator('recovery_dur', {
                initialValue: this.props.initialValues.recovery_dur !== undefined ? this.props.initialValues.recovery_dur : 0,
              })(
                <InputNumber min={0} />,
              )
            }
            <FormattedMessage id="stra.seconds" /> (
            <FormattedMessage id="stra.recovery.dur.help.1" /> {getFieldValue('recovery_dur')} <FormattedMessage id="stra.recovery.dur.help.2" /> )
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="stra.recovery.notify" />}
          >
            {
              getFieldDecorator('recovery_notify', {
                initialValue: this.props.initialValues.recovery_notify === undefined ? false : !this.props.initialValues.recovery_notify,
                valuePropName: 'checked',
              })(
                <Checkbox>
                  <FormattedMessage id="stra.recovery.notify.checkbox" />
                </Checkbox>,
              )
            }
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="stra.period.time" />}
          >
            {
              getFieldDecorator('period_time', {
                initialValue: this.props.initialValues.period_time || PeriodTime.defaultValue,
              })(
                <PeriodTime />,
              )
            }
          </FormItem>
          <FormItem
            {...formItemLayout}
            label={<FormattedMessage id="stra.alert.upgrade" />}
            validateStatus="success" // 兼容
            help="" // 兼容
          >
            {
              getFieldDecorator('alert_upgrade', {
                initialValue: this.props.initialValues.alert_upgrade || AlarmUpgrade.defaultValue,
                rules: [{
                  validator: AlarmUpgrade.checkAlarmUpgrade,
                }],
              })(
                <AlarmUpgrade
                  loading={this.state.notifyDataLoading}
                  notifyGroupData={this.state.notifyGroupData}
                  notifyUserData={this.state.notifyUserData}
                  // eslint-disable-next-line react/jsx-no-bind
                  fetchNotifyData={this.fetchNotifyData.bind(this)}
                />,
              )
            }
          </FormItem>
        </div>
        <FormItem wrapperCol={{ span: 16, offset: 4 }} style={{ marginTop: 24 }}>
          <Button type="primary" htmlType="submit"><FormattedMessage id="form.submit" /></Button>
        </FormItem>
      </Form>
    );
  }
}

export default Form.create()(injectIntl(SettingFields));
