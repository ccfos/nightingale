package dataobj

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	GAUGE    = "GAUGE"
	COUNTER  = "COUNTER"
	SUBTRACT = "SUBTRACT"
	DERIVE   = "DERIVE"
	SPLIT    = "/"
)

const (
	MachineDep   = 1
	MachineIndep = 2
)

type MetricValue struct {
	Nid          string            `json:"nid"`
	Metric       string            `json:"metric"`
	Endpoint     string            `json:"endpoint"`
	Timestamp    int64             `json:"timestamp"`
	Step         int64             `json:"step"`
	ValueUntyped interface{}       `json:"value"`
	Value        float64           `json:"-"`
	CounterType  string            `json:"counterType"`
	Tags         string            `json:"tags"`
	TagsMap      map[string]string `json:"tagsMap"` //保留2种格式，方便后端组件使用
	Extra        string            `json:"extra"`
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
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

func (m *MetricValue) CheckValidity(now int64) (err error) {
	if m == nil {
		err = fmt.Errorf("item is nil")
		return
	}

	if m.Nid == "" && m.Endpoint == "" {
		err = fmt.Errorf("nid or endpoint should not be empty")
		return
	}

	if m.Nid != "" {
		m.Endpoint = NidToEndpoint(m.Nid)
	}

	if m.Metric == "" {
		err = fmt.Errorf("metric should not be empty")
		return
	}

	// 检测保留字
	reservedWords := "[\\t] [\\r] [\\n] [,] [ ] [=]"
	if HasReservedWords(m.Metric) {
		err = fmt.Errorf("metric:%s contains reserved words: %s", m.Metric, reservedWords)
		return
	}
	if HasReservedWords(m.Endpoint) {
		err = fmt.Errorf("endpoint:%s contains reserved words: %s", m.Endpoint, reservedWords)
		return
	}

	if m.CounterType == "" {
		m.CounterType = GAUGE
	}

	if m.CounterType != GAUGE && m.CounterType != COUNTER && m.CounterType != SUBTRACT {
		err = fmt.Errorf("wrong counter type")
		return
	}

	if m.ValueUntyped == "" {
		err = fmt.Errorf("value is nil")
		return
	}

	if m.Step <= 0 {
		err = fmt.Errorf("step should larger than 0")
		return
	}

	if len(m.TagsMap) == 0 {
		m.TagsMap, err = SplitTagsString(m.Tags)
		if err != nil {
			return
		}
	}

	if len(m.TagsMap) > 20 {
		err = fmt.Errorf("tagkv count is too large > 20")
	}

	if len(m.Metric) > 128 {
		err = fmt.Errorf("len(m.Metric) is too large")
		return
	}

	for k, v := range m.TagsMap {
		delete(m.TagsMap, k)
		k = filterString(k)
		v = filterString(v)
		if len(k) == 0 || len(v) == 0 {
			err = fmt.Errorf("tag key and value should not be empty")
			return
		}

		m.TagsMap[k] = v
	}

	m.Tags = SortedTags(m.TagsMap)
	if len(m.Tags) > 512 {
		err = fmt.Errorf("len(m.Tags) is too large")
		return
	}

	//时间超前5分钟则报错
	if m.Timestamp-now > 300 {
		err = fmt.Errorf("point timestamp:%d is ahead of now:%d", m.Timestamp, now)
		return
	}

	if m.Timestamp <= 0 {
		m.Timestamp = now
	}

	m.Timestamp = alignTs(m.Timestamp, int64(m.Step))

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
		err = fmt.Errorf("value [%v] is illegal", m.Value)
		return
	}

	m.Value = vv
	return
}

func HasReservedWords(str string) bool {
	idx := strings.IndexFunc(str, func(r rune) bool {
		return r == '\t' ||
			r == '\r' ||
			r == '\n' ||
			r == ',' ||
			r == ' ' ||
			r == '='
	})
	return idx != -1
}

func filterString(str string) string {
	if -1 == strings.IndexFunc(str,
		func(r rune) bool {
			return r == '\t' ||
				r == '\r' ||
				r == '\n' ||
				r == ',' ||
				r == ' ' ||
				r == '='
		}) {

		return str
	}

	return strings.Map(func(r rune) rune {
		if r == '\t' ||
			r == '\r' ||
			r == '\n' ||
			r == ',' ||
			r == ' ' ||
			r == '=' {
			return '_'
		}
		return r
	}, str)

	return str
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

	result := make(map[string]string)
	tags := strings.Split(s, ",")
	for _, tag := range tags {
		pair := strings.SplitN(tag, "=", 2)
		if len(pair) == 2 {
			result[pair[0]] = pair[1]
		}
	}

	return result
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

func GetCounter(metric, tag string, tagMap map[string]string) (counter string, err error) {
	if tagMap == nil {
		tagMap, err = SplitTagsString(tag)
		if err != nil {
			return
		}
	}

	tagStr := SortedTags(tagMap)
	counter = PKWithTags(metric, tagStr)
	return
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

func (bm *BuiltinMetric) String() string {
	return fmt.Sprintf("%s/%s", bm.Metric, bm.Tags)
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

func (br *BuiltinMetricResponse) String() string {
	return fmt.Sprintf(
		"<Metrics:%v, Checksum:%s, Timestamp:%v>",
		br.Metrics,
		br.Checksum,
		br.Timestamp,
	)
}

type BuiltinMetricSlice []*BuiltinMetric

func (bm BuiltinMetricSlice) Len() int {
	return len(bm)
}
func (bm BuiltinMetricSlice) Swap(i, j int) {
	bm[i], bm[j] = bm[j], bm[i]
}
func (bm BuiltinMetricSlice) Less(i, j int) bool {
	return bm[i].String() < bm[j].String()
}

func alignTs(ts int64, period int64) int64 {
	return ts - ts%period
}
