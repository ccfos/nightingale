package stats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/didi/nightingale/v4/src/common/address"
	"github.com/didi/nightingale/v4/src/common/dataobj"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

var (
	PushUrl string = "http://127.0.0.1:2058/v1/push"
)

func Init(prefix string, addr ...string) {
	if len(addr) > 0 && addr[0] != "" {
		//如果配置了 addr，使用 addr 参数
		PushUrl = addr[0]

	} else if file.IsExist(path.Join(runner.Cwd, "etc", "address.yml")) {
		//address.yml 存在，则使用配置文件的地址
		newAddr := address.GetHTTPAddresses("agentd")
		if len(newAddr) == 0 {
			port := address.GetHTTPPort("agentd")
			PushUrl = fmt.Sprintf("http://127.0.0.1:%d/v1/push", port)
		} else {
			PushUrl = fmt.Sprintf("http://%s/v1/push", newAddr[0])
		}
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
}
