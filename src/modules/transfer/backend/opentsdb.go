package backend

import (
	"bytes"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/pools"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

type OpenTsdbSection struct {
	Enabled     bool   `yaml:"enabled"`
	Name        string `yaml:"name"`
	Batch       int    `yaml:"batch"`
	ConnTimeout int    `yaml:"connTimeout"`
	CallTimeout int    `yaml:"callTimeout"`
	WorkerNum   int    `yaml:"workerNum"`
	MaxConns    int    `yaml:"maxConns"`
	MaxIdle     int    `yaml:"maxIdle"`
	MaxRetry    int    `yaml:"maxRetry"`
	Address     string `yaml:"address"`
}

type OpenTsdbPushEndpoint struct {
	// config
	Section OpenTsdbSection

	OpenTsdbConnPoolHelper *pools.OpenTsdbConnPoolHelper

	// 发送缓存队列 node -> queue_of_data
	OpenTsdbQueue *list.SafeListLimited
}

func (opentsdb *OpenTsdbPushEndpoint) Init() {
	// init connPool
	if opentsdb.Section.Enabled {
		opentsdb.OpenTsdbConnPoolHelper = pools.NewOpenTsdbConnPoolHelper(opentsdb.Section.Address,
			opentsdb.Section.MaxConns, opentsdb.Section.MaxIdle, opentsdb.Section.ConnTimeout,
			opentsdb.Section.CallTimeout)
	}

	// init queue
	if opentsdb.Section.Enabled {
		opentsdb.OpenTsdbQueue = list.NewSafeListLimited(DefaultSendQueueMaxSize)
	}

	// start task
	openTsdbConcurrent := opentsdb.Section.WorkerNum
	if openTsdbConcurrent < 1 {
		openTsdbConcurrent = 1
	}
	go opentsdb.send2OpenTsdbTask(openTsdbConcurrent)

}

// 将原始数据入到tsdb发送缓存队列
func (opentsdb *OpenTsdbPushEndpoint) Push2Queue(items []*dataobj.MetricValue) {
	errCnt := 0
	for _, item := range items {
		tsdbItem := opentsdb.convert2OpenTsdbItem(item)
		isSuccess := opentsdb.OpenTsdbQueue.PushFront(tsdbItem)

		if !isSuccess {
			errCnt += 1
		}
	}
	stats.Counter.Set("opentsdb.queue.err", errCnt)
}

func (opentsdb *OpenTsdbPushEndpoint) send2OpenTsdbTask(concurrent int) {
	batch := opentsdb.Section.Batch // 一次发送,最多batch条数据
	retry := opentsdb.Section.MaxRetry
	addr := opentsdb.Section.Address
	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := opentsdb.OpenTsdbQueue.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(DefaultSendTaskSleepInterval)
			continue
		}
		var openTsdbBuffer bytes.Buffer

		for i := 0; i < count; i++ {
			tsdbItem := items[i].(*dataobj.OpenTsdbItem)
			openTsdbBuffer.WriteString(tsdbItem.OpenTsdbString())
			openTsdbBuffer.WriteString("\n")
			stats.Counter.Set("points.out.opentsdb", 1)
			logger.Debug("send to opentsdb: ", tsdbItem)
		}
		//  同步Call + 有限并发 进行发送
		sema.Acquire()
		go func(addr string, openTsdbBuffer bytes.Buffer, count int) {
			defer sema.Release()

			var err error
			sendOk := false
			for i := 0; i < retry; i++ {
				err = opentsdb.OpenTsdbConnPoolHelper.Send(openTsdbBuffer.Bytes())
				if err == nil {
					sendOk = true
					break
				}
				logger.Warningf("send opentsdb %s fail: %v", addr, err)
				time.Sleep(100 * time.Millisecond)
			}

			if !sendOk {
				stats.Counter.Set("points.out.opentsdb.err", count)
				for _, item := range items {
					logger.Errorf("send %v to opentsdb %s fail: %v", item, addr, err)
				}
			} else {
				logger.Debugf("send to opentsdb %s ok", addr)
			}
		}(addr, openTsdbBuffer, count)
	}
}

func (opentsdb *OpenTsdbPushEndpoint) convert2OpenTsdbItem(d *dataobj.MetricValue) *dataobj.OpenTsdbItem {
	t := dataobj.OpenTsdbItem{Tags: make(map[string]string)}

	for k, v := range d.TagsMap {
		t.Tags[k] = v
	}
	t.Tags["endpoint"] = d.Endpoint
	t.Metric = d.Metric
	t.Timestamp = d.Timestamp
	t.Value = d.Value
	return &t
}
