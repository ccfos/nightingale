import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Card, Input, Tabs, Tooltip, Spin } from 'antd';
import _ from 'lodash';
import moment from 'moment';
import { services } from '@cpts/Graph';
import { GraphData, Hosts } from './interface';
import { prefixCls, metricMap, metricsMeta } from './config';
import { filterMetrics, matchMetrics } from './utils';

interface Props {
  nid?: number,
  hosts: Hosts,
  selectedHosts: Hosts,
  metrics: string[],
  loading: boolean,
  graphs: GraphData[],
  onSelect: (graphData: GraphData) => void, // 指标点击回调
}

interface State {
  searchValue: string,
  activeKey: string,
  metricTipVisible: {
    [index: string]: any,
  },
}

const { TabPane } = Tabs;
function getCurrentMetricMeta(metric: string) {
  if (metricsMeta[metric]) {
    return metricsMeta[metric];
  }
  let currentMetricMeta;
  _.each(metricsMeta, (val, key) => {
    if (key.indexOf('$Name') > -1) {
      const keySplit = key.split('$Name');
      if (metric.indexOf(keySplit[0]) === 0 && metric.indexOf(keySplit[1]) > 0) {
        currentMetricMeta = val;
      }
    }
  });
  return currentMetricMeta;
}
function getSelectedMetricsLen(metric: string, graphs: GraphData[]) {
  const filtered = _.filter(graphs, (o) => {
    return _.find(o.metrics, { selectedMetric: metric });
  });
  if (filtered.length) {
    return <span style={{ color: '#999' }}> +{filtered.length}</span>;
  }
  return null;
}

class MetricSelect extends Component<Props & WrappedComponentProps, State> {
  static defaultProps = {
    nid: undefined,
    hosts: [],
    selectedHosts: [],
    metrics: [],
    graphs: [],
    onSelect: () => {},
  };

  state = {
    searchValue: '',
    activeKey: 'ALL',
    metricTipVisible: {},
  } as State;

  normalizMetrics(key: string) {
    const { metrics } = this.props;
    let newMetrics = _.cloneDeep(metrics);
    if (key !== 'ALL') {
      const { filter, data } = metricMap[key];
      if (filter && filter.type && filter.value) {
        return filterMetrics(filter.type, filter.value, metrics);
      }
      if (data && data.length !== 0) {
        newMetrics = matchMetrics(data, metrics);
        return _.concat([], newMetrics);
      }
      return [];
    }
    return newMetrics;
  }

  dynamicMetricMaps() {
    const { metrics } = this.props;
    return _.filter(metricMap, (val) => {
      const { dynamic, filter } = val;
      if (!dynamic) return true;
      if (filter && filter.type && filter.value) {
        const newMetrics = filterMetrics(filter.type, filter.value, metrics);
        if (newMetrics && newMetrics.length !== 0) {
          return true;
        }
        return false;
      }
      return false;
    });
  }

  handleMetricsSearch = (e: any) => {
    const { value } = e.target;
    this.setState({ searchValue: value });
  }

  handleMetricTabsChange = (key: string) => {
    this.setState({ activeKey: key });
  }

  handleMetricClick = async (metric: string) => {
    const { nid, onSelect, hosts, selectedHosts } = this.props;
    const now = moment();
    const tagkv = await services.fetchTagkv(selectedHosts, metric, hosts);
    const selectedTagkv = _.cloneDeep(tagkv);
    const endpointTagkv = _.find(selectedTagkv, { tagk: 'endpoint' });
    endpointTagkv.tagv = selectedHosts;
    const newGraphConfig = {
      now: now.clone().format('x'),
      start: now.clone().subtract(3600000, 'ms').format('x'),
      end: now.clone().format('x'),
      metrics: [{
        selectedNid: nid,
        selectedEndpoint: selectedHosts,
        endpoints: hosts,
        selectedMetric: metric,
        selectedTagkv,
        tagkv,
        aggrFunc: undefined,
        consolFunc: 'AVERAGE',
        counterList: [],
      }],
    };
    onSelect({
      ...newGraphConfig,
    });
  }

  renderMetricList(metrics: string[] = [], metricTabKey: string) {
    const { graphs } = this.props;
    return (
      <div className="tabPane">
        {
          metrics.length ?
            <ul className="ant-menu ant-menu-vertical ant-menu-root" style={{ border: 'none' }}>
              {
                _.map(metrics, (metric, i) => {
                  return (
                    <li className="ant-menu-item" key={i} onClick={() => { this.handleMetricClick(metric); }}>
                      <Tooltip
                        key={`${metricTabKey}_${metric}`}
                        placement="right"
                        visible={this.state.metricTipVisible[`${metricTabKey}_${metric}`]}
                        // title={() => {
                        //   const currentMetricMeta = getCurrentMetricMeta(metric);
                        //   if (currentMetricMeta) {
                        //     return (
                        //       <div>
                        //         <p>含义：{currentMetricMeta.meaning}</p>
                        //         <p>单位：{currentMetricMeta.unit}</p>
                        //       </div>
                        //     );
                        //   }
                        //   return '';
                        // }}
                        onVisibleChange={(visible) => {
                          const key = `${metricTabKey}_${metric}`;
                          const currentMetricMeta = getCurrentMetricMeta(metric);
                          const { metricTipVisible } = this.state;
                          if (visible && currentMetricMeta) {
                            metricTipVisible[key] = true;
                          } else {
                            metricTipVisible[key] = false;
                          }
                          this.setState({
                            metricTipVisible,
                          });
                        }}
                      >
                        <span>{metric}</span>
                      </Tooltip>
                      {getSelectedMetricsLen(metric, graphs)}
                    </li>
                  );
                })
              }
            </ul> :
            <div style={{ textAlign: 'center' }}>No data</div>
        }
      </div>
    );
  }

  renderMetricTabs() {
    const { searchValue, activeKey } = this.state;
    const metrics = this.normalizMetrics(activeKey);
    let newMetrics = metrics;
    if (searchValue) {
      try {
        const reg = new RegExp(searchValue, 'i');
        newMetrics = _.filter(metrics, (item) => {
          return reg.test(item);
        });
      } catch (e) {
        newMetrics = [];
      }
    }

    const newMetricMap = this.dynamicMetricMaps();
    const tabPanes = _.map(newMetricMap, (val) => {
      const tabName = this.props.intl.locale == 'zh' ? val.alias : val.key;
      return (
        <TabPane tab={tabName} key={val.key}>
          { this.renderMetricList(newMetrics, val.key) }
        </TabPane>
      );
    });
    tabPanes.unshift(
      <TabPane tab={<FormattedMessage id="graph.metric.list.all" />} key="ALL">
        { this.renderMetricList(newMetrics, 'ALL') }
      </TabPane>,
    );

    return (
      <Tabs
        type="card"
        activeKey={activeKey}
        onChange={this.handleMetricTabsChange}
      >
        {tabPanes}
      </Tabs>
    );
  }

  render() {
    return (
      <Spin spinning={this.props.loading}>
        <Card
          className={`${prefixCls}-card`}
          title={
            <span className={`${prefixCls}-metrics-title`}>
              <span><FormattedMessage id="graph.metric.list.title" /></span>
              <Input
                size="small"
                placeholder="Search"
                onChange={this.handleMetricsSearch}
              />
            </span>
          }
        >
          {this.renderMetricTabs()}
        </Card>
      </Spin>
    );
  }
}

export default injectIntl(MetricSelect);
