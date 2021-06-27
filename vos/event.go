package vos

// type History struct {
// 	Key         string            `json:"-"`              // 用于计算event的hashid
// 	Metric      string            `json:"metric"`         // 指标名
// 	Tags        map[string]string `json:"tags,omitempty"` // endpoint/counter
// 	Granularity int               `json:"-"`              // alarm补齐数据时需要
// 	Points      []*HistoryData    `json:"points"`         // 现场值
// }

// type HistoryData struct {
// 	Timestamp int64     `json:"timestamp"`
// 	Value     JsonFloat `json:"value"`
// 	Extra     string    `json:"extra"`
// }

type HistoryPoints struct {
	Metric string            `json:"metric"`
	Tags   map[string]string `json:"tags"`
	Points []*HPoint         `json:"points"`
}

type HPoint struct {
	Timestamp int64     `json:"t"`
	Value     JsonFloat `json:"v"`
}

type HistoryDataS []*HPoint

func (r HistoryDataS) Len() int           { return len(r) }
func (r HistoryDataS) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r HistoryDataS) Less(i, j int) bool { return r[i].Timestamp < r[j].Timestamp }

// func RRDData2HistoryData(datas []*RRDData) []*HistoryData {
// 	count := len(datas)
// 	historyDatas := make([]*HistoryData, 0, count)
// 	for i := count - 1; i >= 0; i-- {
// 		historyData := &HistoryData{
// 			Timestamp: datas[i].Timestamp,
// 			Value:     datas[i].Value,
// 		}
// 		historyDatas = append(historyDatas, historyData)
// 	}

// 	return historyDatas
// }

// func HistoryData2RRDData(datas []*HistoryData) []*RRDData {
// 	count := len(datas)
// 	rrdDatas := make([]*RRDData, 0, count)

// 	for i := count - 1; i >= 0; i-- {
// 		data := &RRDData{
// 			Timestamp: datas[i].Timestamp,
// 			Value:     datas[i].Value,
// 		}
// 		rrdDatas = append(rrdDatas, data)
// 	}
// 	return rrdDatas
// }
