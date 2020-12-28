package dataobj

type QueryData struct {
	Start      int64    `json:"start"`
	End        int64    `json:"end"`
	ConsolFunc string   `json:"consolFunc"`
	Endpoints  []string `json:"endpoints"`
	Nids       []string `json:"nids"`
	Counters   []string `json:"counters" description:"metric/tags"`
	Step       int      `json:"step"`
	DsType     string   `json:"dstype"`
}

type QueryDataForUI struct {
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Metric      string   `json:"metric"`
	Endpoints   []string `json:"endpoints"`
	Nids        []string `json:"nids"`
	Tags        []string `json:"tags"`
	Step        int      `json:"step"`
	DsType      string   `json:"dstype"`
	GroupKey    []string `json:"groupKey"`                               //聚合维度
	AggrFunc    string   `json:"aggrFunc" description:"sum,avg,max,min"` //聚合计算
	ConsolFunc  string   `json:"consolFunc" description:"AVERAGE,MIN,MAX,LAST"`
	Comparisons []int64  `json:"comparisons"` //环比多少时间
}

type QueryDataForUIResp struct {
	Start      int64      `json:"start"`
	End        int64      `json:"end"`
	Endpoint   string     `json:"endpoint"`
	Nid        string     `json:"nid"`
	Counter    string     `json:"counter"`
	DsType     string     `json:"dstype"`
	Step       int        `json:"step"`
	Values     []*RRDData `json:"values"`
	Comparison int64      `json:"comparison"`
}

type QueryDataResp struct {
	Data []*TsdbQueryResponse
	Msg  string
}

// judge 数据层 必须
func (req *QueryData) Key() string {
	return req.Endpoints[0] + "/" + req.Counters[0]
}

func (resp *TsdbQueryResponse) Key() string {
	return resp.Endpoint + "/" + resp.Counter
}

type EndpointsRecv struct {
	Endpoints []string `json:"endpoints"`
	Nids      []string `json:"nids"`
}

type MetricResp struct {
	Metrics []string `json:"metrics"`
}

type EndpointMetricRecv struct {
	Endpoints []string `json:"endpoints"`
	Nids      []string `json:"nids"`
	Metrics   []string `json:"metrics"`
}

type IndexTagkvResp struct {
	Endpoints []string   `json:"endpoints"`
	Nids      []string   `json:"nids"`
	Metric    string     `json:"metric"`
	Tagkv     []*TagPair `json:"tagkv"`
}

type TagPair struct {
	Key    string   `json:"tagk"` // json 和变量不一致为了兼容前端
	Values []string `json:"tagv"`
}

type CludeRecv struct {
	Endpoints []string   `json:"endpoints"`
	Nids      []string   `json:"nids"`
	Metric    string     `json:"metric"`
	Include   []*TagPair `json:"include"`
	Exclude   []*TagPair `json:"exclude"`
}

type XcludeResp struct {
	Endpoint string   `json:"endpoint"`
	Nid      string   `json:"nid"`
	Metric   string   `json:"metric"`
	Tags     []string `json:"tags"`
	Step     int      `json:"step"`
	DsType   string   `json:"dstype"`
}

type IndexByFullTagsRecv struct {
	Endpoints []string  `json:"endpoints"`
	Nids      []string  `json:"nids"`
	Metric    string    `json:"metric"`
	Tagkv     []TagPair `json:"tagkv"`
}

type IndexByFullTagsResp struct {
	Endpoints []string `json:"endpoints"`
	Nids      []string `json:"nids"`
	Metric    string   `json:"metric"`
	Tags      []string `json:"tags"`
	Step      int      `json:"step"`
	DsType    string   `json:"dstype"`
}
