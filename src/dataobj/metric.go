package dataobj

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	GAUGE   = "GAUGE"
	COUNTER = "COUNTER"
	DERIVE  = "DERIVE"
)

type MetricValue struct {
	Metric       string            `json:"metric"`
	Endpoint     string            `json:"endpoint"`
	Timestamp    int64             `json:"timestamp"`
	Step         int64             `json:"step"`
	ValueUntyped interface{}       `json:"value"`
	Value        float64           `json:"-"`
	CounterType  string            `json:"counterType"`
	Tags         string            `json:"tags"`
	TagsMap      map[string]string `json:"tagsMap"` //保留2种格式，方便后端组件使用
}

const SPLIT = "/"

var bufferPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

func (m *MetricValue) String() string {
	return fmt.Sprintf("<MetaData Endpoint:%s, Metric:%s, Timestamp:%d, Step:%d, Value:%v, Tags:%v(%v)>",
		m.Endpoint, m.Metric, m.Timestamp, m.Step, m.ValueUntyped, m.Tags, m.TagsMap)
}

func (m *MetricValue) PK() string {
	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)

	if m.TagsMap == nil || len(m.TagsMap) == 0 {
		ret.WriteString(m.Endpoint)
		ret.WriteString(SPLIT)
		ret.WriteString(m.Metric)

		return ret.String()
	}
	ret.WriteString(m.Endpoint)
	ret.WriteString(SPLIT)
	ret.WriteString(m.Metric)
	ret.WriteString(SPLIT)
	ret.WriteString(SortedTags(m.TagsMap))
	return ret.String()
}

func (m *MetricValue) CheckValidity() (err error) {
	if m == nil {
		err = fmt.Errorf("item is nil")
		return
	}

	//检测保留字
	if HasReservedWords(m.Metric) {
		err = fmt.Errorf("metric:%s contains reserved words:[\\t] [\\r] [\\n] [,] [ ] [=]", m.Metric)
		return
	}

	if HasReservedWords(m.Endpoint) {
		err = fmt.Errorf("endpoint:%s contains reserved words:[\\t] [\\r] [\\n] [,] [ ] [=]", m.Endpoint)
		return
	}

	if m.Metric == "" || m.Endpoint == "" {
		err = fmt.Errorf("Metric|Endpoint is nil")
		return
	}

	if m.CounterType == "" {
		m.CounterType = GAUGE
	}

	if m.CounterType != COUNTER && m.CounterType != GAUGE && m.CounterType != DERIVE {
		err = fmt.Errorf("CounterType error")
		return
	}

	if m.ValueUntyped == "" {
		err = fmt.Errorf("Value is nil")
		return
	}

	if m.Step <= 0 {
		err = fmt.Errorf("step < 0")
		return
	}

	if len(m.TagsMap) == 0 {
		m.TagsMap, err = SplitTagsString(m.Tags)
		if err != nil {
			return
		}
	}

	m.Tags = SortedTags(m.TagsMap)

	if len(m.Metric)+len(m.Tags) > 510 {
		err = fmt.Errorf("metrc+tag > 510 is not illegal:")
		return
	}

	//规范时间戳
	now := time.Now().Unix()
	if m.Timestamp <= 0 || m.Timestamp > now*2 {
		m.Timestamp = now
	}

	valid := true
	var vv float64

	switch cv := m.ValueUntyped.(type) {
	case string:
		vv, err = strconv.ParseFloat(cv, 64)
		if err != nil {
			valid = false
		}
	case float64:
		vv = cv
	case uint64:
		vv = float64(cv)
	case int64:
		vv = float64(cv)
	case int:
		vv = float64(cv)
	default:
		valid = false
	}

	if !valid {
		err = fmt.Errorf("value is not illegal:%v", m)
		return
	}

	m.Value = vv
	return
}

func HasReservedWords(str string) bool {
	if -1 == strings.IndexFunc(str,
		func(r rune) bool {
			return r == '\t' ||
				r == '\r' ||
				r == '\n' ||
				r == ',' ||
				r == ' ' ||
				r == '='
		}) {

		return false
	}

	return true
}

func SortedTags(tags map[string]string) string {
	if tags == nil {
		return ""
	}

	size := len(tags)

	if size == 0 {
		return ""
	}

	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)

	if size == 1 {
		for k, v := range tags {
			ret.WriteString(k)
			ret.WriteString("=")
			ret.WriteString(v)
		}
		return ret.String()
	}

	keys := make([]string, size)
	i := 0
	for k := range tags {
		keys[i] = k
		i++
	}

	sort.Strings(keys)

	for j, key := range keys {
		ret.WriteString(key)
		ret.WriteString("=")
		ret.WriteString(tags[key])
		if j != size-1 {
			ret.WriteString(",")
		}
	}

	return ret.String()
}

func SplitTagsString(s string) (tags map[string]string, err error) {
	err = nil
	tags = make(map[string]string)

	s = strings.Replace(s, " ", "", -1)
	if s == "" {
		return
	}

	tagSlice := strings.Split(s, ",")
	for _, tag := range tagSlice {
		tagPair := strings.SplitN(tag, "=", 2)
		if len(tagPair) == 2 {
			tags[tagPair[0]] = tagPair[1]
		} else {
			err = fmt.Errorf("bad tag %s", tag)
			return
		}
	}

	return
}

func DictedTagstring(s string) map[string]string {
	if s == "" {
		return map[string]string{}
	}
	s = strings.Replace(s, " ", "", -1)

	tag_dict := make(map[string]string)
	tags := strings.Split(s, ",")
	for _, tag := range tags {
		tag_pair := strings.SplitN(tag, "=", 2)
		if len(tag_pair) == 2 {
			tag_dict[tag_pair[0]] = tag_pair[1]
		}
	}
	return tag_dict
}

func PKWithCounter(endpoint, counter string) string {
	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)

	ret.WriteString(endpoint)
	ret.WriteString("/")
	ret.WriteString(counter)

	return ret.String()
}

func PKWithTags(metric, tags string) string {
	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)

	if tags == "" {
		ret.WriteString(metric)
		return ret.String()
	}
	ret.WriteString(metric)
	ret.WriteString("/")
	ret.WriteString(tags)
	return ret.String()
}

func PKWhitEndpointAndTags(endpoint, metric, tags string) string {
	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)

	if tags == "" {
		ret.WriteString(endpoint)
		ret.WriteString("/")
		ret.WriteString(metric)

		return ret.String()
	}
	ret.WriteString(endpoint)
	ret.WriteString("/")
	ret.WriteString(metric)
	ret.WriteString("/")
	ret.WriteString(tags)
	return ret.String()
}

// e.g. tcp.port.listen or proc.num
type BuiltinMetric struct {
	Metric string
	Tags   string
}

func (this *BuiltinMetric) String() string {
	return fmt.Sprintf(
		"%s/%s",
		this.Metric,
		this.Tags,
	)
}

type BuiltinMetricRequest struct {
	Ty       int
	IP       string
	Checksum string
}

type BuiltinMetricResponse struct {
	Metrics   []*BuiltinMetric
	Checksum  string
	Timestamp int64
	ErrCode   int
}

func (this *BuiltinMetricResponse) String() string {
	return fmt.Sprintf(
		"<Metrics:%v, Checksum:%s, Timestamp:%v>",
		this.Metrics,
		this.Checksum,
		this.Timestamp,
	)
}

type BuiltinMetricSlice []*BuiltinMetric

func (this BuiltinMetricSlice) Len() int {
	return len(this)
}
func (this BuiltinMetricSlice) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
}
func (this BuiltinMetricSlice) Less(i, j int) bool {
	return this[i].String() < this[j].String()
}
