package vos

// type TsdbQueryResponse struct {
// 	Start    int64      `json:"start"`
// 	End      int64      `json:"end"`
// 	Endpoint string     `json:"endpoint"`
// 	Nid      string     `json:"nid"`
// 	Counter  string     `json:"counter"`
// 	DsType   string     `json:"dstype"`
// 	Step     int        `json:"step"`
// 	Values   []*RRDData `json:"values"`
// }

// type RRDData struct {
// 	Timestamp int64     `json:"timestamp"`
// 	Value     JsonFloat `json:"value"`
// }

// type RRDValues []*RRDData

// func (r RRDValues) Len() int           { return len(r) }
// func (r RRDValues) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
// func (r RRDValues) Less(i, j int) bool { return r[i].Timestamp < r[j].Timestamp }

// func NewRRDData(ts int64, val float64) *RRDData {
// 	return &RRDData{Timestamp: ts, Value: JsonFloat(val)}
// }

// func (rrd *RRDData) String() string {
// 	return fmt.Sprintf(
// 		"<RRDData:Value:%v TS:%d %v>",
// 		rrd.Value,
// 		rrd.Timestamp,
// 		time.Unix(rrd.Timestamp, 0).Format("2006-01-02 15:04:05"),
// 	)
// }
