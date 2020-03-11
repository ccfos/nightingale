export type GraphId = string | number;
export type DynamicHostsType = '=all' | '=+' | '=-';
export type UpdateType = 'push' | 'unshift' | 'update' | 'allUpdate' | 'delete';
export type GraphDataChangeFunc = (type: UpdateType, id: GraphId, updateConf?: UpdateGrapDataInterface, cbk?: () => void) => void;
export interface ComparisonOption {
  label: string,
  value: string,
}
export type Endpoints = string[];
export interface TagkvInterface {
  tagk: string,
  tagv: string[],
}
export interface CounterInterface {
  id?: string,
  metric: string,
  ns: string,
  step: number,
  counter?: string,
}
export interface PointInterface {
  timestamp: number,
  value: number | null,
  color?: string,
  filledNull?: number | undefined,
  serieOptions: SerieInterface,
  series?: SerieInterface,
}
export interface SerieInterface {
  id: string,
  name: string,
  ns: string,
  metric: string,
  diffNs: string,
  tags: string,
  comparison: number,
  data: PointInterface[],
  lineWidth: number,
  color: string,
  oldColor: string,
  isComparison: boolean,
  isSameNs: boolean,
  isSameMetric: boolean,
  origin: boolean,
  userOptions: any,
}
export type ChartOptionsInterface = any;
export interface MetricInterface {
  selectedNid: number[],
  endpoints: string[],
  selectedEndpoint: string[], // 已选节点
  metrics: string[],
  selectedMetric: string, // 已选指标
  aggrFunc: string, // 聚合方式
  aggrGroup: string[], // 聚合维度 对应 tag
  selectedTagkv: TagkvInterface[], // 已选的 tagkv
  tagkv: TagkvInterface[], // 待选的 tagkv
  counters: CounterInterface[], // 待选的 tagkv
  counterList: CounterInterface[], // 待选的 tagkv
  consolFunc: string,
  tableEmptyText?: string,
}
export interface GraphDataInterface {
  // 基础配置
  id: GraphId,
  title: string,
  type: string,
  now: string, // 记录的当前时间，用于计算出是否自定义时间
  start: string, // 开始时间戳 ms
  end: string, // 结束时间戳 ms
  comparison: string[], // 环比值 (ms eg. 一小时 3600000)
  comparisonOptions: ComparisonOption[], // 环比待选项目
  relativeTimeComparison: boolean, // 相对时间环比
  // 高级配置 -> 用户大盘 todo: 后面会逐步考虑在监控看图也开发这类配置
  tag_id: number, // 大盘分类Id
  fillNull: number,
  threshold: number,
  unit: string, // Y轴单位
  yAxisMin: number,
  yAxisMax: number,
  outerChain: string, // 下钻连接
  subclassId: string | undefined,
  // 特殊配置
  legend: boolean,
  shared: boolean,
  cf: string, // 无用配置，但是必须传递 AVERAGE
  timezoneOffset: string, // 时区偏移，暂时隐藏配置 Asia/shanghai | America/Toronto
  origin: boolean, // 是否开启原始点
  // 多指标配置
  metrics: MetricInterface[],
  xAxis: any,
}
export interface UpdateGrapDataInterface {
  now?: string,
  start?: string,
  end?: string,
  metrics?: MetricInterface[],
  legend?: boolean,
  shared?: boolean,
  sharedSortDirection?: string,
  origin?: boolean,
  comparison?: string[],
  relativeTimeComparison?: boolean,
  comparisonOptions?: ComparisonOption[],
  timezoneOffset?: any, // TODO:
}
