package dataobj

import (
	"fmt"
	"strings"
)

type OpenTsdbItem struct {
	Metric    string            `json:"metric"`
	Tags      map[string]string `json:"tags"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
}

func (t *OpenTsdbItem) String() string {
	return fmt.Sprintf(
		"<Metric:%s, Tags:%v, Value:%v, TS:%d>",
		t.Metric,
		t.Tags,
		t.Value,
		t.Timestamp,
	)
}

func (t *OpenTsdbItem) OpenTsdbString() (s string) {
	s = fmt.Sprintf("put %s %d %.3f ", t.Metric, t.Timestamp, t.Value)

	for k, v := range t.Tags {
		key := strings.ToLower(strings.Replace(k, " ", "_", -1))
		value := strings.Replace(v, " ", "_", -1)
		s += key + "=" + value + " "
	}

	return s
}
