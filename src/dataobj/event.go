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
	Points      []*HistoryData    `json:"points"`         // 现场值
}

type HistoryData struct {
	Timestamp int64     `json:"timestamp"`
	Value     JsonFloat `json:"value"`
	Extra     string    `json:"extra"`
}

func RRDData2HistoryData(datas []*RRDData) []*HistoryData {
	historyDatas := make([]*HistoryData, len(datas))

	for i := range datas {
		historyData := &HistoryData{
			Timestamp: datas[i].Timestamp,
			Value:     datas[i].Value,
		}
		historyDatas[i] = historyData
	}
	return historyDatas
}

func HistoryData2RRDData(datas []*HistoryData) []*RRDData {
	rrdDatas := make([]*RRDData, len(datas))

	for i := range datas {
		data := &RRDData{
			Timestamp: datas[i].Timestamp,
			Value:     datas[i].Value,
		}
		rrdDatas[i] = data
	}
	return rrdDatas
}
