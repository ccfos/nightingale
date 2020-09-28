package dataobj

type IndexModel struct {
	Nid       string            `json:"nid"`
	Endpoint  string            `json:"endpoint"`
	Metric    string            `json:"metric"`
	DsType    string            `json:"dsType"`
	Step      int               `json:"step"`
	Tags      map[string]string `json:"tags"`
	Timestamp int64             `json:"ts"`
}

type IndexResp struct {
	Msg     string
	Total   int
	Invalid int
	Latency int64
}
