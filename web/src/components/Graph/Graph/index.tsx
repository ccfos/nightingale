import React, { Component } from 'react';
import { Spin, Icon } from 'antd';
import moment from 'moment';
import _ from 'lodash';
import D3Graph from '@d3-charts/ts-graph';
import { sortableHandle } from 'react-sortable-hoc';
import '@d3-charts/ts-graph/dist/index.css';
import * as config from '../config';
import * as util from '../util';
import * as services from '../services';
import Legend, { getSerieVisible, getSerieColor, getSerieIndex } from './Legend';
import Title from './Title';
import Extra from './Extra';
import GraphConfigInner from '../GraphConfig/GraphConfigInner';
import { GraphDataInterface, SerieInterface, GraphDataChangeFunc, CounterInterface, ChartOptionsInterface } from '../interface';

interface Props {
  useDragHandle?: boolean,
  data: GraphDataInterface, // 图表数据配置
  height: number, // 图表高度
  graphConfigInnerVisible: boolean, // 内置图表配置栏是否显示
  extraRender: (graph: any) => void, // 图表右侧工具栏扩展
  extraMoreList: React.ReactNode, // 图表右侧工具栏更多选项扩展
  metricMap: any, // 指标信息表，用于设置图表名称
  onChange: GraphDataChangeFunc, // 图表配置修改回调
  onWillInit: (chartOptions: ChartOptionsInterface) => void,
  onDidInit: (chart: any, chartOptions: ChartOptionsInterface) => void,
  onWillUpdate: (chart: any, chartOptions: ChartOptionsInterface) => void,
  onDidUpdate: (chart: any, chartOptions: ChartOptionsInterface) => void,
  onOpenGraphConfig: () => void,
}

interface State {
  spinning: boolean,
  errorText: string | React.ReactNode, // 异常场景下的文案
  series: SerieInterface[],
  isOrigin: boolean,
}

const DragHandle = sortableHandle(() => <Icon type="drag" style={{ cursor: 'move', color: '#999' }} />);

export default class Graph extends Component<Props, State> {
  chart: any;
  graphWrapEle: any;
  static defaultProps = {
    height: 350,
    graphConfigInnerVisible: true,
    extraRender: undefined,
    extraMoreList: undefined,
    metricMap: undefined,
    onChange: _.noop,
    onWillInit: _.noop,
    onDidInit: _.noop,
    onWillUpdate: _.noop,
    onDidUpdate: _.noop,
    onOpenGraphConfig: _.noop,
  };

  xhrs = []; // 保存 xhr 实例，用于组件销毁的时候中断进行中的请求
  chartOptions = config.chart;
  headerHeight = 35;
  counterList = [];
  series = [] as any[];
  state = {
    spinning: false,
    errorText: '',
  } as State;

  componentDidMount() {
    this.fetchData(this.props.data, true, (series: SerieInterface[]) => {
      this.initHighcharts(this.props, series);
    });
  }

  componentWillReceiveProps(nextProps: Props) {
    const nextData = nextProps.data;
    const thisData = this.props.data;
    const selectedNsChanged = !util.isEqualBy(nextData.metrics, thisData.metrics, 'selectedNs');
    const selectedMetricChanged = !util.isEqualBy(nextData.metrics, thisData.metrics, 'selectedMetric');
    const selectedTagkvChanged = !util.isEqualBy(nextData.metrics, thisData.metrics, 'selectedTagkv');
    const aggrFuncChanged = !util.isEqualBy(nextData.metrics, thisData.metrics, 'aggrFunc');
    const consolFuncChanged = !util.isEqualBy(nextData.metrics, thisData.metrics, 'consolFunc');
    const aggrGroupChanged = !util.isEqualBy(nextData.metrics, thisData.metrics, 'aggrGroup');
    const timeChanged = nextData.start !== thisData.start || nextData.end !== thisData.end;

    // 重新加载数据并更新 series
    // 时间范围值、环比值、selectedTagkv值改变的时候需要重新加载数据
    if (
      timeChanged
      || selectedNsChanged
      || selectedMetricChanged
      || selectedTagkvChanged
      || aggrFuncChanged
      || aggrGroupChanged
      || consolFuncChanged
      || !_.isEqual(nextData.comparison, thisData.comparison)
    ) {
      const isFetchCounter = selectedNsChanged || selectedMetricChanged || selectedTagkvChanged;
      this.fetchData(nextProps.data, isFetchCounter, (series: SerieInterface[]) => {
        this.updateHighcharts(nextData, series);
      });
    } else if (
      // 只更新 chartOptions
      nextData.threshold !== thisData.threshold
      || nextData.unit !== thisData.unit
      || nextData.yAxisMax !== thisData.yAxisMax
      || nextData.yAxisMin !== thisData.yAxisMin
      || nextData.timezoneOffset !== thisData.timezoneOffset
      || nextData.shared !== thisData.shared) {
      this.updateHighcharts(nextData);
    }
  }

  componentWillUnmount() {
    // this.abortFetchData();
    if (this.chart) this.chart.destroy();
  }

  getGraphConfig(graphConfig: GraphDataInterface) {
    return {
      ...config.graphDefaultConfig,
      ...graphConfig,
      // eslint-disable-next-line no-nested-ternary
      now: graphConfig.now ? graphConfig.now : graphConfig.end ? graphConfig.end : config.graphDefaultConfig.now,
    };
  }

  getZoomedSeries() {
    return this.series;
  }

  refresh = () => {
    const { data, onChange } = this.props;
    const now = moment();
    const start = (Number(now.format('x')) - Number(data.end)) + Number(data.start) + '';
    const end = now.format('x');

    onChange('update', data.id, {
      start, end, now: end,
    } as GraphDataInterface);
  }

  resize = () => {
    if (this.chart && this.chart.resizeHandle) {
      this.chart.resizeHandle();
    }
  }

  async fetchData(graphConfig: GraphDataInterface, isFetchCounter: boolean, cbk: (series: SerieInterface[]) => void) {
    graphConfig = this.getGraphConfig(graphConfig);

    // this.abortFetchData();

    this.setState({ spinning: true });
    let { metrics } = graphConfig;

    try {
      const metricsResult = await services.normalizeMetrics(metrics, this.props.graphConfigInnerVisible);
      // eslint-disable-next-line prefer-destructuring
      metrics = metricsResult.metrics;

      if (metricsResult.canUpdate) {
        this.props.onChange('update', graphConfig.id, {
          metrics,
        } as GraphDataInterface);
        // 临时图场景，只是更新 tagkv, 这块需要再优化下
        // return;
      }
      if (isFetchCounter) {
        this.counterList = await services.fetchCounterList(metrics);
      }

      const endpointCounters = util.normalizeEndpointCounters(graphConfig, this.counterList);
      const errorText = this.checkEndpointCounters(endpointCounters, config.countersMaxLength);

      if (!errorText) {
        // get series
        const sourceData = await services.getHistory(endpointCounters);
        this.series = util.normalizeSeries(sourceData, graphConfig);
      }

      if (cbk) cbk(this.series);
      this.setState({ errorText, spinning: false });
    } catch (e) {
      console.log(e);
      if (e.statusText === 'abort') return;

      let errorText = e.err;

      if (e.statusText === 'error') {
        errorText = 'The network has been disconnected, please check the network';
      } else if (e.statusText === 'Not Found') {
        errorText = '404 Not Found';
      } else if (e.responseJSON) {
        errorText = _.get(e.responseJSON, 'msg', e.responseText);

        if (!errorText || e.status === 500) {
          errorText = 'Data loading exception, please refresh and reload';
        }

        // request entity too large
        if (e.status === 413) {
          errorText = 'Request condition is too large, please reduce the condition';
        }
      }

      this.setState({ errorText, spinning: false });
    }
  }

  // eslint-disable-next-line class-methods-use-this
  checkEndpointCounters(endpointCounters: CounterInterface[], countersMaxLength: number) {
    let errorText: any = '';
    if (!_.get(endpointCounters, 'length', 0)) {
      errorText = 'No data';
    }

    if (endpointCounters.length > countersMaxLength) {
      errorText = (
        <span className="counters-maxLength">
          Too many series，Current
          {endpointCounters.length}
          cap
          {countersMaxLength}
          ，Please reduce the number of series
        </span>
      );
    }

    return errorText;
  }

  // abortFetchData() {
  //   _.each(this.xhrs, (xhr) => {
  //     if (_.isFunction(_.get(xhr, 'abort'))) xhr.abort();
  //   });
  //   this.xhrs = [];
  // }

  initHighcharts(props: Props, series?: SerieInterface[]) {
    const graphConfig = this.getGraphConfig(props.data);
    const chartOptions = {
      timestamp: 'x',
      chart: {
        height: props.height,
        renderTo: this.graphWrapEle,
      },
      xAxis: graphConfig.xAxis,
      yAxis: util.getYAxis({}, graphConfig),
      tooltip: {
        shared: graphConfig.shared,
        formatter: (points: any[]) => {
          return util.getTooltipsContent({
            points,
            chartWidth: this.graphWrapEle.offsetWidth - 40,
            comparison: graphConfig.comparison,
            isComparison: !!_.get(graphConfig.comparison, 'length'),
          });
        },
      },
      series,
      legend: {
        enabled: false,
      },
      onZoom: (getZoomedSeries: any) => {
        this.getZoomedSeries = getZoomedSeries;
        this.forceUpdate();
      },
    };

    if (!this.chart) {
      this.props.onWillInit(chartOptions);
      this.chart = new D3Graph(chartOptions);
      this.props.onDidInit(this.chart, chartOptions);
    }
  }

  updateHighcharts(graphConfig = this.props.data, series = this.series) {
    if (!this.chart) {
      this.initHighcharts(this.props);
      return;
    }
    graphConfig = this.getGraphConfig(graphConfig);

    const updateChartOptions = {
      yAxis: util.getYAxis(this.chart.options.yAxis, graphConfig),
      tooltip: {
        xAxis: graphConfig.xAxis,
        shared: graphConfig.shared,
        formatter: (points: any[]) => {
          return util.getTooltipsContent({
            points,
            chartWidth: this.graphWrapEle.offsetWidth - 40,
            comparison: graphConfig.comparison,
            isComparison: !!_.get(graphConfig.comparison, 'length'),
          });
        },
      },
      series,
    };

    this.props.onWillUpdate(this.chart, updateChartOptions);
    this.chart.update(updateChartOptions);
    this.props.onDidUpdate(this.chart, updateChartOptions);
  }

  handleLegendRowSelectedChange = (selectedKeys: string[], highlightedKeys: string[]) => {
    const series = this.getZoomedSeries()

    this.series = _.map(series, (serie, i) => {
      const oldColor = _.get(serie, 'oldColor', serie.color);
      return {
        ...serie,
        visible: getSerieVisible(serie, selectedKeys),
        zIndex: getSerieIndex(serie, highlightedKeys, series.length, i),
        color: getSerieColor(serie, highlightedKeys, oldColor),
        oldColor,
      };
    });

    this.updateHighcharts();
  }

  render() {
    const { spinning, errorText } = this.state;
    const { height, onChange, extraRender, data } = this.props;
    const graphConfig = this.getGraphConfig(data);

    return (
      <div className={graphConfig.legend ? 'graph-container graph-container-hasLegend' : 'graph-container'}>
        <div
          className="graph-header"
          style={{
            height: this.headerHeight,
            lineHeight: `${this.headerHeight}px`,
          }}
        >
          <div className="graph-extra">
            <div style={{ display: 'inline-block' }}>
              {
                this.props.useDragHandle ? <DragHandle /> : null
              }
              {
                _.isFunction(extraRender)
                  ? extraRender(this) :
                  <Extra
                    graphConfig={graphConfig}
                    counterList={this.counterList}
                    onOpenGraphConfig={this.props.onOpenGraphConfig}
                    moreList={this.props.extraMoreList}
                  />
              }
            </div>
          </div>
          <Title
            title={data.title}
            selectedMetric={_.get(graphConfig.metrics, '[0].selectedMetric')}
          />
        </div>
        {
          this.props.graphConfigInnerVisible
            ? <GraphConfigInner
              data={graphConfig}
              onChange={onChange}
            /> : null
        }
        <Spin spinning={spinning}>
          <div style={{ height, display: !errorText ? 'none' : 'block' }}>
            {
              errorText ?
                <div className="graph-errorText">
                  {errorText}
                </div> : null
            }
          </div>
          <div
            className="graph-content"
            ref={(ref) => { this.graphWrapEle = ref; }}
            style={{
              height,
              backgroundColor: '#fff',
              display: errorText ? 'none' : 'block',
            }}
          />
        </Spin>
        <Legend
          style={{ display: graphConfig.legend ? 'block' : 'none' }}
          series={this.getZoomedSeries()}
          onSelectedChange={this.handleLegendRowSelectedChange}
          comparisonOptions={graphConfig.comparisonOptions}
        />
      </div>
    );
  }
}
