package cache

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

func ReportEndpoint() {
	sema := semaphore.NewSemaphore(10)
	for {
		endpoints := NewEndpoints.PopBackBy(500)
		if len(endpoints) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		sema.Acquire()
		go func(endpoints []interface{}) {
			defer sema.Release()
			reportEndpoint(endpoints)
		}(endpoints)
	}
}

type reportRes struct {
	Err string `json:"err"`
	Dat string `json:"dat"`
}

func reportEndpoint(endpoints []interface{}) {
	for {
		addrs := address.GetHTTPAddresses("monapi")
		perm := rand.Perm(len(addrs))
		for i := range perm {
			url := fmt.Sprintf("http://%s/v1/portal/endpoint", addrs[perm[i]])
			m := map[string][]interface{}{
				"endpoints": endpoints,
			}

			var body reportRes
			err := httplib.Post(url).
				JSONBodyQuiet(m).
				SetTimeout(3*time.Second).
				Header("x-srv-token", "monapi-builtin-token").
				ToJSON(&body)
			if err != nil {
				logger.Warningf("curl [%s] fail: %v. retry", url, err)
				stats.Counter.Set("report.endpoint.err", 1)
				continue
			}

			// 数据库连接出错会出现此情况
			if body.Err != "" {
				logger.Warningf("curl [%s] fail: %v. retry", url, body.Err)
				stats.Counter.Set("report.endpoint.err", 1)
				continue
			}

			// 推送成功，将 endpoint 状态标记为已上报，避免下次 index 重启时再重新上报
			for _, endpoint := range endpoints {
				metricIndexMap, _ := IndexDB.GetMetricIndexMap(endpoint.(string))
				metricIndexMap.SetReported()
			}
			return

		}
		time.Sleep(100 * time.Millisecond)
	}
}
