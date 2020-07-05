import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import PropTypes from 'prop-types';
import update from 'react-addons-update';
import { Row, Col, Spin, Table, Form, Select, Input, InputNumber, Icon, TreeSelect, DatePicker } from 'antd';
import _ from 'lodash';
import moment from 'moment';
import { normalizeTreeData, renderTreeNodes } from '@cpts/Layout/utils';
import request from '@common/request';
import api from '@common/api';
import Tagkv from './Tagkv';
import Comparison from './Comparison';
import * as config from '../config';
import { getTimeLabelVal } from '../util';
import hasDtag from '../util/hasDtag';
import * as services from '../services';
import { GraphDataInterface, MetricInterface, TagkvInterface } from '../interface';

interface Props {
  data: GraphDataInterface,
  isScreen: boolean,
  subclassOptions: any[],
  btnDisable: (loading: boolean) => void,
}

interface State {
  graphConfig: GraphDataInterface,
  loading: boolean,
  tableEmptyText: string,
  nsSearchVal: string,
  counterListVisible: boolean,
  advancedVisible: boolean,
  treeData: any[] | undefined,
  originTreeData: any[] | undefined,
}

const FormItem = Form.Item;
const { Option } = Select;
function normalizeMetrics(metrics: MetricInterface[]) {
  if (_.isEmpty(metrics)) {
    return [{
      key: _.uniqueId('METRIC_'),
      selectedNid: undefined,
      selectedMetric: '',
    }];
  }
  return _.map(metrics, metric => ({
    ...metric,
    key: metric.selectedMetric || _.uniqueId('METRIC_'),
  }));
}

function intersectionTagkv(selectedTagkv: TagkvInterface[], tagkv: TagkvInterface[]) {
  return _.intersectionBy(selectedTagkv, tagkv, 'tagk');
}

export default class GraphConfigForm extends Component<Props, State> {
  static contextTypes = {
    getSelectedNode: PropTypes.func,
    habitsId: PropTypes.string,
  };

  static defaultProps = {
    data: {},
    isScreen: false,
    subclassOptions: [],
  };

  constructor(props: Props) {
    super(props);
    const { data } = props;
    const metrics = normalizeMetrics(data.metrics);
    this.state = {
      graphConfig: {
        ...config.graphDefaultConfig,
        ...props.data,
        metrics,
      },
      loading: false,
      tableEmptyText: 'No data',
      nsSearchVal: '', // 节点搜索值
      counterListVisible: false,
      advancedVisible: false,
      treeData: undefined,
      originTreeData: undefined,
    } as State;
  }

  componentDidMount() {
    this.fetchTreeData(() => {
      this.fetchAllByMetric();
    });
  }

  setLoading(loading: boolean) {
    this.setState({ loading });
    this.props.btnDisable(loading);
  }

  getColumns() {
    return [
      {
        title: '曲线',
        dataIndex: 'counter',
      }, {
        title: '周期',
        dataIndex: 'step',
        width: 45,
        render(text: string) {
          return <span>{text}{'s'}</span>;
        },
      },
    ];
  }

  fetchTreeData(cbk?: () => void) {
    request(api.tree).then((res) => {
      const treeData = normalizeTreeData(res);
      this.setState({ treeData, originTreeData: res }, () => {
        if (cbk) cbk();
      });
    });
  }

  async fetchAllByMetric() {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(metrics[0]);
    const currentMetricObjIndex = 0;

    if (currentMetricObj) {
      try {
        this.setLoading(true);
        if (currentMetricObj.selectedNid !== undefined) {
          await this.fetchEndpoints(currentMetricObj);
          if (!_.isEmpty(currentMetricObj.selectedEndpoint)) {
            await this.fetchMetrics(currentMetricObj);
            if (currentMetricObj.selectedMetric) {
              await this.fetchTagkv(currentMetricObj);
              if (currentMetricObj.selectedTagkv) {
                await this.fetchCounterList(currentMetricObj);
              }
            }
          }
        }
        this.setState(update(this.state, {
          graphConfig: {
            metrics: {
              $splice: [
                [currentMetricObjIndex, 1, currentMetricObj],
              ],
            },
          },
        }));
        this.setLoading(false);
      } catch (e) {
        this.setLoading(false);
      }
    }
  }

  async fetchEndpoints(metricObj: MetricInterface) {
    try {
      const endpoints = await services.fetchEndPoints(metricObj.selectedNid as any);
      let selectedEndpoint = metricObj.selectedEndpoint || ['=all'];
      if (!hasDtag(selectedEndpoint)) {
        selectedEndpoint = _.intersection(endpoints, metricObj.selectedEndpoint);
      }
      metricObj.endpoints = endpoints;
      metricObj.selectedEndpoint = selectedEndpoint;
      return metricObj;
    } catch (e) {
      return e;
    }
  }

  async fetchMetrics(metricObj: MetricInterface) {
    try {
      const metricList = await services.fetchMetrics(metricObj.selectedEndpoint, metricObj.endpoints);
      const selectedMetric = _.indexOf(metricList, metricObj.selectedMetric) > -1 ? metricObj.selectedMetric : '';
      metricObj.metrics = metricList;
      metricObj.selectedMetric = selectedMetric;
      return metricObj;
    } catch (e) {
      return e;
    }
  }

  async fetchTagkv(metricObj: MetricInterface) {
    try {
      const tagkv = await services.fetchTagkv(metricObj.selectedEndpoint, metricObj.selectedMetric, metricObj.endpoints);
      let selectedTagkv = metricObj.selectedTagkv || _.chain(tagkv).map(item => ({ tagk: item.tagk, tagv: ['=all'] })).value();
      if (!hasDtag(selectedTagkv)) {
        selectedTagkv = intersectionTagkv(metricObj.selectedTagkv, tagkv);
      }

      metricObj.tagkv = tagkv;
      metricObj.selectedTagkv = selectedTagkv;
    } catch (e) {
      return e;
    }
  }

  async fetchCounterList(metricObj: MetricInterface) {
    try {
      const counterList = await services.fetchCounterList([{
        selectedEndpoint: metricObj.selectedEndpoint,
        selectedMetric: metricObj.selectedMetric,
        selectedTagkv: metricObj.selectedTagkv,
        tagkv: metricObj.tagkv,
      }]);
      metricObj.counterList = counterList;
    } catch (e) {
      return e;
    }
  }

  handleCommonFieldChange = (changedObj: any) => {
    const newChangedObj: any = {};
    _.each(changedObj, (val, key) => {
      newChangedObj[key] = {
        $set: val,
      };
    });
    this.setState(update(this.state, {
      graphConfig: newChangedObj,
    }));
  }

  handleNsChange = async (selectedNid: number[], currentMetricObj: MetricInterface) => {
    try {
      this.setLoading(true);
      currentMetricObj.selectedNid = selectedNid;
      if (selectedNid !== undefined) {
        await this.fetchEndpoints(currentMetricObj);
        if (!_.isEmpty(currentMetricObj.selectedEndpoint)) {
          await this.fetchMetrics(currentMetricObj);
          if (currentMetricObj.selectedMetric) {
            await this.fetchTagkv(currentMetricObj);
            if (currentMetricObj.selectedTagkv) {
              await this.fetchCounterList(currentMetricObj);
            }
          }
        }
      } else {
        // delete ns
        currentMetricObj.endpoints = [];
        currentMetricObj.selectedEndpoint = [];
        currentMetricObj.metrics = [];
        currentMetricObj.selectedMetric = '';
        currentMetricObj.tagkv = [];
        currentMetricObj.selectedTagkv = [];
        currentMetricObj.counterList = [];
      }

      this.setState(update(this.state, {
        graphConfig: {
          metrics: {
            $splice: [
              [0, 1, currentMetricObj],
            ],
          },
        },
      }));
      this.setLoading(false);
    } catch (e) {
      console.error(e);
      this.setLoading(false);
    }
  }

  handleEndpointChange = async (selectedEndpoint: string[]) => {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(metrics[0]);
    const currentMetricObjIndex = 0;

    if (currentMetricObj) {
      try {
        this.setLoading(true);
        currentMetricObj.selectedEndpoint = selectedEndpoint;
        const endpointTagkv = _.find(currentMetricObj.selectedTagkv, { tagk: 'endpoint' });
        if (endpointTagkv) {
          endpointTagkv.tagv = selectedEndpoint;
        } else {
          currentMetricObj.selectedTagkv = [
            ...currentMetricObj.selectedTagkv || [],
            {
              tagk: 'endpoint',
              tagv: selectedEndpoint,
            },
          ];
        }
        if (!_.isEmpty(currentMetricObj.selectedEndpoint)) {
          await this.fetchMetrics(currentMetricObj);
          if (currentMetricObj.selectedMetric) {
            await this.fetchTagkv(currentMetricObj);
            if (currentMetricObj.selectedTagkv) {
              await this.fetchCounterList(currentMetricObj);
            }
          }
        } else {
          currentMetricObj.metrics = [];
          currentMetricObj.selectedMetric = '';
          currentMetricObj.tagkv = [];
          currentMetricObj.selectedTagkv = [];
          currentMetricObj.counterList = [];
        }

        this.setState(update(this.state, {
          graphConfig: {
            metrics: {
              $splice: [
                [currentMetricObjIndex, 1, currentMetricObj],
              ],
            },
          },
        }));
        this.setLoading(false);
      } catch (e) {
        console.error(e);
        this.setLoading(false);
      }
    }
  }

  handleMetricChange = async (selectedMetric: string, currentMetric: string) => {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(_.find(metrics, { selectedMetric: currentMetric }));
    const currentMetricObjIndex = _.findIndex(metrics, { selectedMetric: currentMetric });

    if (currentMetricObj) {
      try {
        this.setLoading(true);
        currentMetricObj.selectedMetric = selectedMetric;
        if (selectedMetric) {
          await this.fetchTagkv(currentMetricObj);
          if (currentMetricObj.selectedTagkv) {
            await this.fetchCounterList(currentMetricObj);
          }
        } else {
          currentMetricObj.tagkv = [];
          currentMetricObj.selectedTagkv = [];
          currentMetricObj.counterList = [];
        }

        this.setState(update(this.state, {
          graphConfig: {
            metrics: {
              $splice: [
                [currentMetricObjIndex, 1, currentMetricObj],
              ],
            },
          },
        }));
        this.setLoading(false);
      } catch (e) {
        console.error(e);
        this.setLoading(false);
      }
    }
  }

  handleTagkvChange = async (currentMetric: string, tagk: string, tagv: string[]) => {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(_.find(metrics, { selectedMetric: currentMetric }));
    const currentMetricObjIndex = _.findIndex(metrics, { selectedMetric: currentMetric });
    const currentTagIndex = _.findIndex(currentMetricObj!.selectedTagkv, { tagk });

    if (currentTagIndex > -1) {
      if (!tagv.length) { // 删除
        currentMetricObj!.selectedTagkv = update(currentMetricObj!.selectedTagkv, {
          $splice: [
            [currentTagIndex, 1],
          ],
        });
      } else { // 修改
        currentMetricObj!.selectedTagkv = update(currentMetricObj!.selectedTagkv, {
          $splice: [
            [currentTagIndex, 1, {
              tagk, tagv,
            }],
          ],
        });
      }
    } else if (tagv.length) { // 添加
      currentMetricObj!.selectedTagkv = update(currentMetricObj!.selectedTagkv, {
        $push: [{
          tagk, tagv,
        }],
      });
    }
    this.setState(update(this.state, {
      graphConfig: {
        metrics: {
          $splice: [
            [currentMetricObjIndex, 1, currentMetricObj],
          ],
        },
      },
    }));
    try {
      this.setLoading(true);
      await this.fetchCounterList(currentMetricObj!);
      this.setLoading(false);
    } catch (e) {
      console.error(e);
      this.setLoading(false);
    }
  }

  handleAggregateChange = (currentMetric: string, value: string) => {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(_.find(metrics, { selectedMetric: currentMetric }));
    const currentMetricObjIndex = _.findIndex(metrics, { selectedMetric: currentMetric });

    currentMetricObj!.aggrFunc = value;
    this.setState(update(this.state, {
      graphConfig: {
        metrics: {
          $splice: [
            [currentMetricObjIndex, 1, currentMetricObj],
          ],
        },
      },
    }));
  }

  handleconsolFuncChange = (currentMetric: string, value: string) => {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(_.find(metrics, { selectedMetric: currentMetric }));
    const currentMetricObjIndex = _.findIndex(metrics, { selectedMetric: currentMetric });

    currentMetricObj!.consolFunc = value;
    this.setState(update(this.state, {
      graphConfig: {
        metrics: {
          $splice: [
            [currentMetricObjIndex, 1, currentMetricObj],
          ],
        },
      },
    }));
  }

  handleAggregateDimensionChange = (currentMetric: string, value: string[]) => {
    const { metrics } = this.state.graphConfig;
    const currentMetricObj = _.cloneDeep(_.find(metrics, { selectedMetric: currentMetric }));
    const currentMetricObjIndex = _.findIndex(metrics, { selectedMetric: currentMetric });

    currentMetricObj!.aggrGroup = value;
    this.setState(update(this.state, {
      graphConfig: {
        metrics: {
          $splice: [
            [currentMetricObjIndex, 1, currentMetricObj],
          ],
        },
      },
    }));
  }

  handleSubclassChange = (val: string) => {
    this.setState(update(this.state, {
      graphConfig: {
        subclassId: {
          $set: val,
        },
      },
    }));
  }

  handleTitleChange = (e: any) => {
    this.setState(update(this.state, {
      graphConfig: {
        title: {
          $set: e.target.value,
        },
      },
    }));
  }

  handleTimeOptionChange = (val: string) => {
    const now = moment();
    let { start, end } = this.state.graphConfig;

    if (val !== 'custom') {
      start = now.clone().subtract(Number(val), 'ms').format('x');
      end = now.format('x');
    } else {
      start = moment(Number(start)).format('x');
      end = moment().format('x');
    }
    this.setState(update(this.state, {
      graphConfig: {
        start: {
          $set: start,
        },
        end: {
          $set: end,
        },
        now: {
          $set: end,
        },
      },
    }));
  }

  handleDateChange = (key: string, d: moment.Moment | null) => {
    const val = moment.isMoment(d) ? d.format('x') : null;
    this.setState(update(this.state, {
      graphConfig: {
        [key]: {
          $set: val,
        },
      },
    }));
  }

  handleThresholdChange = (val: number | undefined) => {
    this.setState(update(this.state, {
      graphConfig: {
        threshold: {
          $set: val,
        },
      },
    }));
  }

  renderMetrics() {
    const { getSelectedNode } = this.context;
    const selectedNode = getSelectedNode();
    const { metrics } = this.state.graphConfig;
    const metricObj = metrics[0]; // 当前只支持一个指标
    const currentMetric = metricObj.selectedMetric;
    const withoutEndpointTagkv = _.filter(metricObj.tagkv, item => item.tagk !== 'endpoint');
    const treeDefaultExpandedKeys = !_.isEmpty(metricObj.selectedNid) ? metricObj.selectedNid : [selectedNode.id];
    const aggrGroupOptions = _.map(_.get(metrics, '[0].tagkv'), item => ({ label: item.tagk, value: item.tagk }));
    return (
      <div>
        <FormItem
          labelCol={{ span: 3 }}
          wrapperCol={{ span: 21 }}
          label={<FormattedMessage id="graph.config.node" />}
          style={{ marginBottom: 5 }}
          required
        >
          <TreeSelect
            showSearch
            allowClear
            treeDefaultExpandedKeys={_.map(treeDefaultExpandedKeys, _.toString)}
            treeNodeFilterProp="title"
            treeNodeLabelProp="path"
            dropdownStyle={{ maxHeight: 200, overflow: 'auto' }}
            value={metricObj.selectedNid}
            onChange={value => this.handleNsChange(value, metricObj)}
          >
            {renderTreeNodes(this.state.treeData)}
          </TreeSelect>
        </FormItem>
        <Tagkv
          type="modal"
          data={[{
            tagk: 'endpoint',
            tagv: metricObj.endpoints,
          }]}
          selectedTagkv={[{
            tagk: 'endpoint',
            tagv: metricObj.selectedEndpoint,
          }]}
          onChange={(tagk, tagv) => { this.handleEndpointChange(tagv); }}
          renderItem={(tagk, tagv, selectedTagv, show) => {
            return (
              <Input
                readOnly
                value={_.join(_.slice(selectedTagv, 0, 40), ', ')}
                size="default"
                // placeholder="若无此tag，请留空"
                onClick={() => {
                  show(tagk);
                }}
              />
            );
          }}
          wrapInner={(content, tagk) => {
            return (
              <FormItem
                key={tagk}
                labelCol={{ span: 3 }}
                wrapperCol={{ span: 21 }}
                label={tagk}
                style={{ marginBottom: 5 }}
                className="graph-tags"
                required
              >
                {content}
              </FormItem>
            );
          }}
        />
        <FormItem
          labelCol={{ span: 3 }}
          wrapperCol={{ span: 21 }}
          label={<FormattedMessage id="graph.config.metric" />}
          style={{ marginBottom: 5 }}
          required
        >
          <Select
            showSearch
            size="default"
            style={{ width: '100%' }}
            // placeholder="监控项指标名, 如cpu.idle"
            // notFoundContent="请输入关键词过滤"
            className="select-metric"
            value={metricObj.selectedMetric}
            onChange={(value: string) => this.handleMetricChange(value, currentMetric)}
          >
            {
              _.map(metricObj.metrics, o => <Option key={o}>{o}</Option>)
            }
          </Select>
        </FormItem>
        <Row style={{ marginBottom: 5 }}>
          <Col span={12}>
            <FormItem
              labelCol={{ span: 6 }}
              wrapperCol={{ span: 18 }}
              label={<FormattedMessage id="graph.config.aggr" />}
              style={{ marginBottom: 0 }}
            >
              <Select
                allowClear
                size="default"
                style={{ width: '100%' }}
                // placeholder="无"
                value={metricObj.aggrFunc}
                onChange={(val: string) => this.handleAggregateChange(currentMetric, val)}
              >
                <Option value="sum"><FormattedMessage id="graph.config.aggr.sum" /></Option>
                <Option value="avg"><FormattedMessage id="graph.config.aggr.avg" /></Option>
                <Option value="max"><FormattedMessage id="graph.config.aggr.max" /></Option>
                <Option value="min"><FormattedMessage id="graph.config.aggr.min" /></Option>
              </Select>
            </FormItem>
          </Col>
          <Col span={12}>
            <FormItem
              labelCol={{ span: 5 }}
              wrapperCol={{ span: 19 }}
              label={<FormattedMessage id="graph.config.aggr.group" />}
              style={{ marginBottom: 0 }}
            >
              <Select
                mode="multiple"
                size="default"
                style={{ width: '100%' }}
                disabled={!metricObj.aggrFunc}
                // placeholder="无"
                value={metricObj.aggrGroup || []}
                onChange={(val: string[]) => this.handleAggregateDimensionChange(currentMetric, val)}
              >
                {
                  _.map(aggrGroupOptions, o => <Option key={o.value} value={o.value}>{o.label}</Option>)
                }
              </Select>
            </FormItem>
          </Col>
        </Row>
        {/* <FormItem
          labelCol={{ span: 3 }}
          wrapperCol={{ span: 21 }}
          label="采样函数"
          style={{ marginBottom: 0 }}
        >
          <Select
            allowClear
            size="default"
            style={{ width: '100%' }}
            placeholder="无"
            value={metricObj.consolFunc}
            onChange={(val: string) => this.handleconsolFuncChange(currentMetric, val)}
          >
            <Option value="AVERAGE">均值</Option>
            <Option value="MAX">最大值</Option>
            <Option value="MIN">最小值</Option>
          </Select>
        </FormItem> */}
        <Tagkv
          type="modal"
          data={withoutEndpointTagkv}
          selectedTagkv={metricObj.selectedTagkv}
          onChange={(tagk, tagv) => { this.handleTagkvChange(currentMetric, tagk, tagv); }}
          renderItem={(tagk, tagv, selectedTagv, show) => {
            return (
              <Input
                readOnly
                value={_.join(_.slice(selectedTagv, 0, 40), ', ')}
                size="default"
                // placeholder="若无此tag，请留空"
                onClick={() => {
                  show(tagk);
                }}
              />
            );
          }}
          wrapInner={(content, tagk) => {
            return (
              <FormItem
                key={tagk}
                labelCol={{ span: 3 }}
                wrapperCol={{ span: 21 }}
                label={tagk}
                style={{ marginBottom: 5 }}
                className="graph-tags"
                required
              >
                {content}
              </FormItem>
            );
          }}
        />
        <FormItem
          labelCol={{ span: 3 }}
          wrapperCol={{ span: 21 }}
          label={<FormattedMessage id="graph.config.series" />}
          style={{ marginBottom: 5 }}
        >
          <span style={{ color: '#ff7f00', paddingRight: 5 }}>
            {_.get(metricObj.counterList, 'length')}
            <FormattedMessage id="graph.config.series.unit" />
          </span>
          <a onClick={() => {
            this.setState({ counterListVisible: !this.state.counterListVisible });
          }}>
            <Icon type={
              this.state.counterListVisible ? 'circle-o-up' : 'circle-o-down'
            } />
          </a>
          {
            this.state.counterListVisible &&
            <Table
              bordered={false}
              size="middle"
              columns={this.getColumns()}
              dataSource={metricObj.counterList}
              locale={{
                emptyText: metricObj.tableEmptyText,
              }}
            />
          }
        </FormItem>
      </div>
    );
  }

  render() {
    const { loading, graphConfig } = this.state;
    const { now, start, end } = graphConfig;
    const timeVal = now === end ? getTimeLabelVal(start, end, 'value') : 'custom';
    const datePickerStartVal = moment(Number(start)).format(config.timeFormatMap.moment);
    const datePickerEndVal = moment(Number(end)).format(config.timeFormatMap.moment);

    return (
      <Spin spinning={loading}>
        <Form>
          {
            this.props.isScreen ?
              <FormItem
                labelCol={{ span: 3 }}
                wrapperCol={{ span: 21 }}
                label={<FormattedMessage id="graph.config.cate" />}
                style={{ marginBottom: 5 }}
                required
              >
                <Select
                  style={{ width: '100%' }}
                  value={graphConfig.subclassId}
                  onChange={this.handleSubclassChange}
                >
                  {
                    _.map(this.props.subclassOptions, (option) => {
                      return <Option key={option.id} value={option.id}>{option.name}</Option>;
                    })
                  }
                </Select>
              </FormItem> : null
          }
          <FormItem
            labelCol={{ span: 3 }}
            wrapperCol={{ span: 21 }}
            label={<FormattedMessage id="graph.config.graph.title" />}
            style={{ marginBottom: 5 }}
          >
            <Input
              style={{ width: '100%' }}
              value={graphConfig.title}
              onChange={this.handleTitleChange}
              placeholder="The metric name as the default title"
            />
          </FormItem>
          <FormItem
            labelCol={{ span: 3 }}
            wrapperCol={{ span: 21 }}
            label={<FormattedMessage id="graph.config.time" />}
            style={{ marginTop: 5, marginBottom: 0 }}
            required
            >
            <Select size="default" style={
              timeVal === 'custom' ?
                {
                  width: 198,
                  marginRight: 10,
                } : {
                  width: '100%',
                }
            }
              value={timeVal}
              onChange={this.handleTimeOptionChange}
            >
              {
                _.map(config.time, o => <Option key={o.value} value={o.value}><FormattedMessage id={o.label} /></Option>)
              }
            </Select>
            {
              timeVal === 'custom' ?
                [
                  <DatePicker
                    key="datePickerStart"
                    format={config.timeFormatMap.moment}
                    style={{
                      position: 'relative',
                      width: 193,
                      minWidth: 193,
                    }}
                    defaultValue={moment(datePickerStartVal)}
                    onOk={d => this.handleDateChange('start', d)}
                  />,
                  <span key="datePickerDivider" style={{ paddingLeft: 10, paddingRight: 10 }}>-</span>,
                  <DatePicker
                    key="datePickerEnd"
                    format={config.timeFormatMap.moment}
                    style={{
                      position: 'relative',
                      width: 194,
                      minWidth: 194,
                    }}
                    defaultValue={moment(datePickerEndVal)}
                    onOk={d => this.handleDateChange('end', d)}
                  />,
                ] : false
            }
          </FormItem>
          <FormItem
            labelCol={{ span: 3 }}
            wrapperCol={{ span: 21 }}
            label={<FormattedMessage id="graph.config.comparison" />}
            style={{ marginBottom: 0 }}
            >
            <Comparison
              size="default"
              comparison={graphConfig.comparison}
              relativeTimeComparison={graphConfig.relativeTimeComparison}
              comparisonOptions={graphConfig.comparisonOptions}
              graphConfig={graphConfig}
              onChange={(values: any) => {
                this.handleCommonFieldChange({
                  start: values.start,
                  end: values.end,
                  now: values.now,
                  comparison: values.comparison,
                  relativeTimeComparison: values.relativeTimeComparison,
                  comparisonOptions: values.comparisonOptions,
                });
              }}
            />
          </FormItem>
          {this.renderMetrics()}
          <FormItem
            labelCol={{ span: 3 }}
            wrapperCol={{ span: 21 }}
            label={<FormattedMessage id="graph.config.threshold" />}
            style={{ marginBottom: 5 }}
          >
            <InputNumber
              style={{ width: '100%' }}
              value={graphConfig.threshold}
              onChange={this.handleThresholdChange}
            />
          </FormItem>
        </Form>
      </Spin>
    );
  }
}
