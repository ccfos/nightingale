package influxdb

import (
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"

	client "github.com/influxdata/influxdb/client/v2"
	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

type InfluxdbSection struct {
	Enabled   bool   `yaml:"enabled"`
	Name      string `yaml:"name"`
	Batch     int    `yaml:"batch"`
	MaxRetry  int    `yaml:"maxRetry"`
	WorkerNum int    `yaml:"workerNum"`
	Timeout   int    `yaml:"timeout"`
	Address   string `yaml:"address"`
	Database  string `yaml:"database"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Precision string `yaml:"precision"`
}

type InfluxdbDataSource struct {
	// config
	Section               InfluxdbSection
	SendQueueMaxSize      int
	SendTaskSleepInterval time.Duration

	// 发送缓存队列 node -> queue_of_data
	InfluxdbQueue *list.SafeListLimited
}

func (influxdb *InfluxdbDataSource) Init() {

	// init queue
	if influxdb.Section.Enabled {
		influxdb.InfluxdbQueue = list.NewSafeListLimited(influxdb.SendQueueMaxSize)
	}

	// init task
	influxdbConcurrent := influxdb.Section.WorkerNum
	if influxdbConcurrent < 1 {
		influxdbConcurrent = 1
	}
	go influxdb.send2InfluxdbTask(influxdbConcurrent)
}

// 将原始数据插入到influxdb缓存队列
func (influxdb *InfluxdbDataSource) Push2Queue(items []*dataobj.MetricValue) {
	errCnt := 0
	for _, item := range items {
		influxdbItem := influxdb.convert2InfluxdbItem(item)
		isSuccess := influxdb.InfluxdbQueue.PushFront(influxdbItem)

		if !isSuccess {
			errCnt += 1
		}
	}
	stats.Counter.Set("influxdb.queue.err", errCnt)
}

func (influxdb *InfluxdbDataSource) send2InfluxdbTask(concurrent int) {
	batch := influxdb.Section.Batch // 一次发送,最多batch条数据
	retry := influxdb.Section.MaxRetry
	addr := influxdb.Section.Address
	sema := semaphore.NewSemaphore(concurrent)

	var err error
	c, err := NewInfluxdbClient(influxdb.Section)
	defer c.Client.Close()

	if err != nil {
		logger.Errorf("init influxdb client fail: %v", err)
		return
	}

	for {
		items := influxdb.InfluxdbQueue.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(influxdb.SendTaskSleepInterval)
			continue
		}

		influxdbItems := make([]*dataobj.InfluxdbItem, count)
		for i := 0; i < count; i++ {
			influxdbItems[i] = items[i].(*dataobj.InfluxdbItem)
			stats.Counter.Set("points.out.influxdb", 1)
			logger.Debug("send to influxdb: ", influxdbItems[i])
		}

		//  同步Call + 有限并发 进行发送
		sema.Acquire()
		go func(addr string, influxdbItems []*dataobj.InfluxdbItem, count int) {
			defer sema.Release()
			sendOk := false

			for i := 0; i < retry; i++ {
				err = c.Send(influxdbItems)
				if err == nil {
					sendOk = true
					break
				}
				logger.Warningf("send influxdb fail: %v", err)
				time.Sleep(time.Millisecond * 10)
			}

			if !sendOk {
				stats.Counter.Set("points.out.influxdb.err", count)
				logger.Errorf("send %v to influxdb %s fail: %v", influxdbItems, addr, err)
			} else {
				logger.Debugf("send to influxdb %s ok", addr)
			}
		}(addr, influxdbItems, count)
	}
}

func (influxdb *InfluxdbDataSource) convert2InfluxdbItem(d *dataobj.MetricValue) *dataobj.InfluxdbItem {
	t := dataobj.InfluxdbItem{Tags: make(map[string]string), Fields: make(map[string]interface{})}

	for k, v := range d.TagsMap {
		t.Tags[k] = v
	}
	t.Tags["endpoint"] = d.Endpoint
	t.Measurement = d.Metric
	t.Fields["value"] = d.Value
	t.Timestamp = d.Timestamp

	return &t
}

type InfluxClient struct {
	Client    client.Client
	Database  string
	Precision string
}

func NewInfluxdbClient(section InfluxdbSection) (*InfluxClient, error) {
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     section.Address,
		Username: section.Username,
		Password: section.Password,
		Timeout:  time.Millisecond * time.Duration(section.Timeout),
	})

	if err != nil {
		return nil, err
	}

	return &InfluxClient{
		Client:    c,
		Database:  section.Database,
		Precision: section.Precision,
	}, nil
}

func (c *InfluxClient) Send(items []*dataobj.InfluxdbItem) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  c.Database,
		Precision: c.Precision,
	})
	if err != nil {
		logger.Error("create batch points error: ", err)
		return err
	}

	for _, item := range items {
		pt, err := client.NewPoint(item.Measurement, item.Tags, item.Fields, time.Unix(item.Timestamp, 0))
		if err != nil {
			logger.Error("create new points error: ", err)
			continue
		}
		bp.AddPoint(pt)
	}

	return c.Client.Write(bp)
}

func (influxdb *InfluxdbDataSource) GetInstance(metric, endpoint string, tags map[string]string) []string {
	// influxdb 单实例 或 influx-proxy
	return []string{influxdb.Section.Address}
}
