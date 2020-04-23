import PropTypes from 'prop-types';
import moment from 'moment';

const now = moment();
export const comparisonOptions = [
  {
    label: '1小时',
    labelEn: '1 hour',
    value: '3600000',
  }, {
    label: '2小时',
    labelEn: '2 hours',
    value: '7200000',
  }, {
    label: '1天',
    labelEn: '1 day',
    value: '86400000',
  }, {
    label: '2天',
    labelEn: '2 days',
    value: '172800000',
  }, {
    label: '7天',
    labelEn: '7 days',
    value: '604800000',
  },
];

export const graphPropTypes = {
  // 基础配置
  title: PropTypes.string,
  type: PropTypes.string,
  now: PropTypes.oneOfType([
    PropTypes.string,
    PropTypes.number,
  ]), // 记录的当前时间，用于计算出是否自定义时间
  start: PropTypes.oneOfType([
    PropTypes.string,
    PropTypes.number,
  ]), // 开始时间戳 ms
  end: PropTypes.oneOfType([
    PropTypes.string,
    PropTypes.number,
  ]), // 结束时间戳 ms
  comparison: PropTypes.arrayOf(PropTypes.string), // 环比值 (ms eg. 一小时 3600000)
  comparisonOptions: PropTypes.array, // 环比待选项目
  relativeTimeComparison: PropTypes.bool, // 相对时间环比
  // 高级配置 -> 用户大盘 todo: 后面会逐步考虑在监控看图也开发这类配置
  tag_id: PropTypes.number, // 大盘分类Id
  fillNull: PropTypes.number,
  threshold: PropTypes.number,
  unit: PropTypes.string, // Y轴单位
  yAxisMin: PropTypes.number,
  yAxisMax: PropTypes.number,
  outerChain: PropTypes.string, // 下钻连接
  // 特殊配置
  legend: PropTypes.bool,
  shared: PropTypes.bool,
  cf: PropTypes.string, // 无用配置，但是必须传递 AVERAGE
  timezoneOffset: PropTypes.string, // 时区偏移，暂时隐藏配置 Asia/shanghai | America/Toronto
  origin: PropTypes.bool, // 是否开启原始点
  // 多指标配置
  metrics: PropTypes.arrayOf(
    PropTypes.shape({
      selectedEndpoint: PropTypes.array, // 已选节点
      selectedMetric: PropTypes.string, // 已选指标
      aggrFunc: PropTypes.string, // 聚合方式
      aggrGroup: PropTypes.array, // 聚合维度 对应 tag
      selectedTagkv: PropTypes.array, // 已选的 tagkv
      tagKv: PropTypes.array, // 待选的 tagkv
      counters: PropTypes.array, // 待选的 tagkv
    }),
  ),
};

export const graphDefaultConfig = {
  title: '',
  type: 'chart',
  now: now.clone().format('x'),
  start: now.clone().subtract(3600000, 'ms').format('x'),
  end: now.clone().format('x'),
  comparisonOptions,
  threshold: undefined,
  legend: false,
  shared: false,
  metrics: [
    {
      selectedEndpoint: [],
      selectedMetric: '',
      aggrFunc: undefined,
      aggrGroup: [],
      selectedTagkv: [],
      consolFunc: 'AVERAGE',
    },
  ],
};

export const hexPalette = [
  '#3399CC', '#CC9933', '#9966CC', '#66CC66', '#CC3333', '#99CCCC', '#CCCC66',
  '#CC99CC', '#99CC99', '#CC6666', '#336699', '#996633', '#993399', '#339966', '#993333',
];

export const chart = {
  chart: {
    zoomType: 'x',
    marginRight: 10,
    marginTop: 1,
    marginBottom: 30,
    height: 350,
    animation: false,
    ignoreHiddenSeries: false,
  },
  time: {
    // timezoneOffset: 5 * 60,
  },
  title: {
    align: 'left',
    x: 0,
    style: {
      color: '#666',
      fontSize: 12,
    },
  },
  credits: {
    enabled: false,
  },
  xAxis: {
    labels: {
      enabled: true,
      style: {
        color: '#999',
        fontSize: 11,
      },
    },
  },
  yAxis: {
    title: {
      text: '',
    },
    opposite: false,
    gridLineColor: '#f1f1f1',
    labels: {
      align: 'left',
      x: 0,
      style: {
        fontSize: 11,
        color: '#999',
        // 'text-shadow': detect.browser.safari ? '1px 1px 1px #fff' : '-1px 0 #fff,0 1px #fff,1px 0 #fff,0 -1px #fff',
      },
    },
  },
  scrollbar: {
    enabled: false,
  },
  rangeSelector: {
    enabled: false,
  },
  exporting: {
    enabled: false,
  },
  navigator: {
    enabled: false,
  },
  plotOptions: {
    series: {
      animation: false,
      turboThreshold: 0,
      dataGrouping: {
        enabled: false,
      },
    },
  },
  tooltip: {
    dateTimeLabelFormats: {
      millisecond: '%Y-%m-%d %H:%M:%S',
      second: '%Y-%m-%d %H:%M:%S',
      minute: '%Y-%m-%d %H:%M:%S',
      hour: '%Y-%m-%d %H:%M:%S',
      day: '%Y-%m-%d %H:%M:%S',
      week: '%Y-%m-%d %H:%M:%S',
      month: '%Y-%m-%d %H:%M:%S',
      year: '%Y-%m-%d %H:%M:%S',
    },
    animation: false,
    valueDecimals: 3,
    backgroundColor: null,
    borderWidth: 0,
    shadow: false,
    useHTML: true,
    shared: false,
    split: false,
  },
  series: [],
};

export const time: { [index: string]: string }[] = [
  {
    label: '1小时',
    value: '3600000',
  }, {
    label: '2小时',
    value: '7140000', // 7200000 - 60000 避免边界问题导致的 刷新页面 效果不一致(step不一致)，JIRA #2566
  }, {
    label: '1天',
    value: '86400000',
  }, {
    label: '2天',
    value: '172800000',
  }, {
    label: '7天',
    value: '604800000',
  }, {
    label: '30天',
    value: '2592000000',
  }, {
    label: '其它',
    value: 'custom',
  },
];

export const aggrOptions = [
  {
    label: '求和',
    value: 'sum',
  }, {
    label: '均值',
    value: 'avg',
  }, {
    label: '最大值',
    value: 'max',
  }, {
    label: '最小值',
    value: 'min',
  },
];

export const timeFormatMap = {
  moment: 'YYYY-MM-DD HH:mm:ss',
  antd: 'yyyy-MM-dd HH:mm:ss',
};

export const countersMaxLength = 2000;

export const counterListPropType = PropTypes.arrayOf(PropTypes.shape({ // 用于拉取 history 接口的数据
  ns: PropTypes.string,
  metric: PropTypes.string,
  counter: PropTypes.string,
}));
