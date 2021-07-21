package vos

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/didi/nightingale/v5/pkg/istr"
)

const (
	SPLIT = "/"
)

type MetricPoint struct {
	PK           string            `json:"pk"`     // 内部字段，ident、metric、sorted(tags)拼接之后算md5
	Ident        string            `json:"ident"`  // 资源标识，跟资源无关的监控数据，该字段为空
	Alias        string            `json:"alias"`  // 资源名称，跟资源无关的监控数据，该字段为空
	Metric       string            `json:"metric"` // 监控指标名称
	TagsMap      map[string]string `json:"tags"`   // 监控数据标签
	TagsLst      []string          `json:"-"`      // 内部字段，用于对TagsMap排序
	Time         int64             `json:"time"`   // 时间戳，单位是秒
	ValueUntyped interface{}       `json:"value"`  // 监控数据数值，可以是int float string，但最终要能转换为float64
	Value        float64           `json:"-"`      // 内部字段，最终转换之后的float64数值
}

func (m *MetricPoint) Tidy(now int64) error {
	if m == nil {
		return fmt.Errorf("point is nil")
	}

	// 时间超前5分钟则报错
	if m.Time-now > 300 {
		return fmt.Errorf("point_time(%d) - server_time(%d) = %d. use ntp to calibrate host time?", m.Time, now, m.Time-now)
	}

	// 时间延迟30分钟则报错
	if m.Time-now < -1800 {
		return fmt.Errorf("point_time(%d) - server_time(%d) = %d. use ntp to calibrate host time?", m.Time, now, m.Time-now)
	}

	if m.Time <= 0 {
		m.Time = now
	}

	if m.Metric == "" {
		return fmt.Errorf("metric is blank")
	}

	if istr.SampleKeyInvalid(m.Metric) {
		return fmt.Errorf("metric:%s contains reserved words", m.Metric)
	}

	if istr.SampleKeyInvalid(m.Ident) {
		return fmt.Errorf("ident:%s contains reserved words", m.Ident)
	}

	if m.ValueUntyped == nil {
		return fmt.Errorf("value is nil")
	}

	safemap := make(map[string]string)
	for k, v := range m.TagsMap {
		if istr.SampleKeyInvalid(k) {
			return fmt.Errorf("tag key: %s contains reserved words", k)
		}

		if len(k) == 0 {
			return fmt.Errorf("tag key is blank, metric: %s", m.Metric)
		}

		v = strings.Map(func(r rune) rune {
			if r == '\t' ||
				r == '\r' ||
				r == '\n' ||
				r == ',' {
				return '_'
			}
			return r
		}, v)

		if len(v) == 0 {
			safemap[k] = "nil"
		} else {
			safemap[k] = v
		}
	}

	m.TagsMap = safemap

	valid := true
	var vv float64
	var err error

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
		return fmt.Errorf("value(%v) is illegal", m.Value)
	}

	m.Value = vv

	return nil
}

// func DictedTagstring(s string) map[string]string {
// 	if i := strings.Index(s, " "); i != -1 {
// 		s = strings.Replace(s, " ", "", -1)
// 	}

// 	rmap := make(map[string]string)

// 	if s == "" {
// 		return rmap
// 	}

// 	tags := strings.Split(s, ",")
// 	for _, tag := range tags {
// 		pair := strings.SplitN(tag, "=", 2)
// 		if len(pair) != 2 {
// 			continue
// 		}

// 		if pair[0] == "" {
// 			continue
// 		}

// 		if pair[1] == "" {
// 			rmap[pair[0]] = "nil"
// 		} else {
// 			rmap[pair[0]] = pair[1]
// 		}
// 	}

// 	return rmap
// }

func DictedTagList(tags []string) map[string]string {
	rmap := make(map[string]string)
	if len(tags) == 0 {
		return rmap
	}

	for _, tag := range tags {
		pair := strings.SplitN(tag, "=", 2)
		if len(pair) != 2 {
			continue
		}

		if pair[0] == "" {
			continue
		}

		if pair[1] == "" {
			rmap[pair[0]] = "nil"
		} else {
			rmap[pair[0]] = pair[1]
		}
	}

	return rmap
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
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
