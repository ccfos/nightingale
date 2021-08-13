package vos

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
