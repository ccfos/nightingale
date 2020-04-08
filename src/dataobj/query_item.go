package dataobj

type QueryData struct {
	Start      int64    `json:"start"`
	End        int64    `json:"end"`
	ConsolFunc string   `json:"consolFunc"`
	Endpoints  []string `json:"endpoints"`
	Counters   []string `json:"counters"`
	Step       int      `json:"step"`
	DsType     string   `json:"dstype"`
}

type QueryDataForUI struct {
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Metric      string   `json:"metric"`
	Endpoints   []string `json:"endpoints"`
	Tags        []string `json:"tags"`
	Step        int      `json:"step"`
	DsType      string   `json:"dstype"`
	GroupKey    []string `json:"groupKey"` //聚合维度
	AggrFunc    string   `json:"aggrFunc"` //聚合计算
	ConsolFunc  string   `json:"consolFunc"`
	Comparisons []int64  `json:"comparisons"` //环比多少时间
}

type QueryDataForUIResp struct {
	Start      int64      `json:"start"`
	End        int64      `json:"end"`
	Endpoint   string     `json:"endpoint"`
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
