package dataobj

type InfluxdbItem struct {
	Measurement string                 `json:"metric"`
	Tags        map[string]string      `json:"tags"`
	Fileds      map[string]interface{} `json:"fileds"`
	Timestamp   int64                  `json:"timestamp"`
}