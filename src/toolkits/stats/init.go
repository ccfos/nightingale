package stats

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/didi/nightingale/src/dataobj"

	"github.com/toolkits/pkg/logger"
)

var (
	PushUrl string = "http://127.0.0.1:2058/api/collector/push"
)

func Init(prefix string, addr ...string) {
	if len(addr) > 0 {
		PushUrl = addr[0]
	}

	Counter = NewCounter(prefix)
	go Push()
}

func Push() {
	t1 := time.NewTicker(time.Duration(10) * time.Second)
	for {
		<-t1.C
		counters := Counter.Dump()
		items := []*dataobj.MetricValue{}
		for metric, value := range counters {
			items = append(items, NewMetricValue(metric, int64(value)))
		}

		push(items)
	}
}

func NewMetricValue(metric string, value int64) *dataobj.MetricValue {
	item := &dataobj.MetricValue{
		Metric:       metric,
		Timestamp:    time.Now().Unix(),
		ValueUntyped: value,
		CounterType:  "GAUGE",
		Step:         10,
	}
	return item
}

func push(items []*dataobj.MetricValue) {
	bs, err := json.Marshal(items)
	if err != nil {
		logger.Warning(err)
		return
	}

	bf := bytes.NewBuffer(bs)

	resp, err := http.Post(PushUrl, "application/json", bf)
	if err != nil {
		logger.Warning(err)
		return
	}

	defer resp.Body.Close()
	return
}
