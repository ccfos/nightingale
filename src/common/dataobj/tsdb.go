package dataobj

import (
	"fmt"
	"math"
	"time"

	"github.com/didi/nightingale/src/toolkits/str"
)

type JsonFloat float64

func (v JsonFloat) MarshalJSON() ([]byte, error) {
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return []byte("null"), nil
	} else {
		return []byte(fmt.Sprintf("%f", f)), nil
	}
}

type RRDData struct {
	Timestamp int64     `json:"timestamp"`
	Value     JsonFloat `json:"value"`
}

type RRDValues []*RRDData

func (r RRDValues) Len() int           { return len(r) }
func (r RRDValues) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r RRDValues) Less(i, j int) bool { return r[i].Timestamp < r[j].Timestamp }

func NewRRDData(ts int64, val float64) *RRDData {
	return &RRDData{Timestamp: ts, Value: JsonFloat(val)}
}

func (rrd *RRDData) String() string {
	return fmt.Sprintf(
		"<RRDData:Value:%v TS:%d %v>",
		rrd.Value,
		rrd.Timestamp,
		time.Unix(rrd.Timestamp, 0).Format("2006-01-02 15:04:05"),
	)
}

type TsdbQueryResponse struct {
	Start    int64      `json:"start"`
	End      int64      `json:"end"`
	Endpoint string     `json:"endpoint"`
	Nid      string     `json:"nid"`
	Counter  string     `json:"counter"`
	DsType   string     `json:"dstype"`
	Step     int        `json:"step"`
	Values   []*RRDData `json:"values"`
}

type TsdbItem struct {
	Nid       string            `json:"nid"`
	Endpoint  string            `json:"endpoint"`
	Metric    string            `json:"metric"`
	Tags      string            `json:"tags"`
	TagsMap   map[string]string `json:"tagsMap"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	DsType    string            `json:"dstype"`
	Step      int               `json:"step"`
	Heartbeat int               `json:"heartbeat"`
	Min       string            `json:"min"`
	Max       string            `json:"max"`
	From      int               `json:"from"`
}

const GRAPH = 1

func (t *TsdbItem) String() string {
	return fmt.Sprintf(
		"<Endpoint:%s, Metric:%s, Tags:%v, TagsMap:%v, Value:%v, TS:%d %v DsType:%s, Step:%d, Heartbeat:%d, Min:%s, Max:%s>",
		t.Endpoint,
		t.Metric,
		t.Tags,
		t.TagsMap,
		t.Value,
		t.Timestamp,
		str.UnixTsFormat(t.Timestamp),
		t.DsType,
		t.Step,
		t.Heartbeat,
		t.Min,
		t.Max,
	)
}

func (t *TsdbItem) PrimaryKey() string {
	return str.PK(t.Endpoint, t.Metric, t.Tags)
}

func (t *TsdbItem) MD5() string {
	return str.MD5(t.Endpoint, t.Metric, str.SortedTags(t.TagsMap))
}

func (t *TsdbItem) UUID() string {
	return str.UUID(t.Endpoint, t.Metric, t.Tags, t.DsType, t.Step)
}

// ConsolFunc 是RRD中的概念，比如：MIN|MAX|AVERAGE
type TsdbQueryParam struct {
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	ConsolFunc string `json:"consolFunc"`
	Nid        string `json:"nid"`
	Endpoint   string `json:"endpoint"`
	Counter    string `json:"counter"`
	Step       int    `json:"step"`
	DsType     string `json:"dsType"`
}

func (g *TsdbQueryParam) PK() string {
	return PKWithCounter(g.Endpoint, g.Counter)
}

func NidToEndpoint(nid string) string {
	endpoint := "__nid__" + nid + "__"
	return endpoint
}
