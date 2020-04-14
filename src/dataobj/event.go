package dataobj

// Event 传递到alarm的结构体, 尽可能少的字段, 发出通知需要的信息由alarm自己补全
type Event struct {
	ID        string    `json:"-"`
	Sid       int64     `json:"sid"`
	EventType string    `json:"event_type"` // alert/recover
	Hashid    uint64    `json:"hashid"`     // 全局唯一 根据counter计算
	Etime     int64     `json:"etime"`
	Endpoint  string    `json:"endpoint"`
	History   []History `json:"-"`
	Detail    string    `json:"detail"`
	Info      string    `json:"info"`
	Value     string    `json:"value"`
	Partition string    `json:"-"`
}

type History struct {
	Key         string            `json:"-"`              // 用于计算event的hashid
	Metric      string            `json:"metric"`         // 指标名
	Tags        map[string]string `json:"tags,omitempty"` // endpoint/counter
	Granularity int               `json:"-"`              // alarm补齐数据时需要
	Points      []*RRDData        `json:"points"`         // 现场值
	Extra       string            `json:"extra"`
}
