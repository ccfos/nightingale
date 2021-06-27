package backend

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	pc "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
	"go.uber.org/atomic"

	"github.com/didi/nightingale/v5/vos"
)

const (
	DefaultPopNum = 1000
)

type PromeSection struct {
	Enable                       bool           `yaml:"enable"`
	Name                         string         `yaml:"name"`
	Batch                        int            `yaml:"batch"`
	MaxRetry                     int            `yaml:"maxRetry"`
	LookbackDeltaMinute          int            `yaml:"lookbackDeltaMinute"`
	MaxConcurrentQuery           int            `yaml:"maxConcurrentQuery"`
	MaxSamples                   int            `yaml:"maxSamples"`
	MaxFetchAllSeriesLimitMinute int64          `yaml:"maxFetchAllSeriesLimitMinute"`
	RemoteWrite                  []RemoteConfig `yaml:"remoteWrite"`
	RemoteRead                   []RemoteConfig `yaml:"remoteRead"`
}

type RemoteConfig struct {
	Name                string `yaml:"name"`
	Url                 string `yaml:"url"`
	RemoteTimeoutSecond int    `yaml:"remoteTimeoutSecond"`
}

type PromeDataSource struct {
	Section     PromeSection
	LocalTmpDir string
	// 除了promql的查询，需要后端存储
	Queryable storage.SampleAndChunkQueryable
	// promql相关查询
	QueryEngine  *promql.Engine
	PushQueue    *list.SafeListLimited
	WriteTargets []*HttpClient
}
type safePromQLNoStepSubqueryInterval struct {
	value atomic.Int64
}

type HttpClient struct {
	remoteName string // Used to differentiate clients in metrics.
	url        *url.URL
	Client     *http.Client
	timeout    time.Duration
}

func durationToInt64Millis(d time.Duration) int64 {
	return int64(d / time.Millisecond)
}
func (i *safePromQLNoStepSubqueryInterval) Set(ev model.Duration) {
	i.value.Store(durationToInt64Millis(time.Duration(ev)))
}
func (i *safePromQLNoStepSubqueryInterval) Get(int64) int64 {
	return i.value.Load()
}
func (pd *PromeDataSource) CleanUp() {
	err := os.RemoveAll(pd.LocalTmpDir)
	logger.Infof("[remove_prome_tmp_dir_err][dir:%+v][err: %v]", pd.LocalTmpDir, err)

}
func (pd *PromeDataSource) Init() {
	// 模拟创建本地存储目录
	dbDir, err := ioutil.TempDir("", "tsdb-api-ready")
	if err != nil {
		logger.Errorf("[error_create_local_tsdb_dir][err: %v]", err)
		return
	}
	pd.LocalTmpDir = dbDir

	promlogConfig := promlog.Config{}
	// 使用本地目录创建remote-storage
	remoteS := remote.NewStorage(promlog.New(&promlogConfig), prometheus.DefaultRegisterer, func() (int64, error) {
		return 0, nil
	}, dbDir, 1*time.Minute, nil)

	// ApplyConfig 加载queryables
	remoteReadC := make([]*pc.RemoteReadConfig, 0)
	for _, u := range pd.Section.RemoteRead {

		ur, err := url.Parse(u.Url)
		if err != nil {
			logger.Errorf("[prome_ds_init_error][parse_url_error][url:%+v][err:%+v]", u.Url, err)
			continue
		}

		remoteReadC = append(remoteReadC,
			&pc.RemoteReadConfig{
				URL:           &config_util.URL{URL: ur},
				RemoteTimeout: model.Duration(time.Duration(u.RemoteTimeoutSecond) * time.Second),
				ReadRecent:    true,
			},
		)
	}
	if len(remoteReadC) == 0 {
		logger.Errorf("[prome_ds_error_got_zero_remote_read_storage]")
		return
	}
	err = remoteS.ApplyConfig(&pc.Config{RemoteReadConfigs: remoteReadC})
	if err != nil {
		logger.Errorf("[error_load_remote_read_config][err: %v]", err)
		return
	}
	pLogger := log.NewNopLogger()

	noStepSubqueryInterval := &safePromQLNoStepSubqueryInterval{}

	queryQueueDir, err := ioutil.TempDir(dbDir, "prom_query_concurrency")
	opts := promql.EngineOpts{
		Logger:                   log.With(pLogger, "component", "query engine"),
		Reg:                      prometheus.DefaultRegisterer,
		MaxSamples:               pd.Section.MaxSamples,
		Timeout:                  30 * time.Second,
		ActiveQueryTracker:       promql.NewActiveQueryTracker(queryQueueDir, pd.Section.MaxConcurrentQuery, log.With(pLogger, "component", "activeQueryTracker")),
		LookbackDelta:            time.Duration(pd.Section.LookbackDeltaMinute) * time.Minute,
		NoStepSubqueryIntervalFn: noStepSubqueryInterval.Get,
		EnableAtModifier:         true,
	}

	queryEngine := promql.NewEngine(opts)
	pd.QueryEngine = queryEngine
	pd.Queryable = remoteS

	// 初始化writeClients
	if len(pd.Section.RemoteWrite) == 0 {
		logger.Warningf("[prome_ds_init_with_zero_RemoteWrite_target]")
		logger.Infof("[successfully_init_prometheus_datasource][remote_read_num:%+v][remote_write_num:%+v]",
			len(pd.Section.RemoteRead),
			len(pd.Section.RemoteWrite),
		)
		return
	}
	writeTs := make([]*HttpClient, 0)
	for _, u := range pd.Section.RemoteWrite {
		ur, err := url.Parse(u.Url)
		if err != nil {
			logger.Errorf("[prome_ds_init_error][parse_url_error][url:%+v][err:%+v]", u.Url, err)
			continue
		}
		writeTs = append(writeTs,
			&HttpClient{
				remoteName: u.Name,
				url:        ur,
				Client:     &http.Client{},
				timeout:    time.Duration(u.RemoteTimeoutSecond) * time.Second,
			})
	}
	pd.WriteTargets = writeTs
	// 开启prometheus 队列消费协程
	go pd.remoteWrite()
	logger.Infof("[successfully_init_prometheus_datasource][remote_read_num:%+v][remote_write_num:%+v]",
		len(remoteReadC),
		len(writeTs),
	)
}

func (pd *PromeDataSource) Push2Queue(points []*vos.MetricPoint) {
	for _, point := range points {
		pt, err := pd.convertOne(point)
		if err != nil {
			logger.Errorf("[prome_convertOne_error][point: %+v][err:%s]", point, err)
			continue
		}
		ok := pd.PushQueue.PushFront(pt)
		if !ok {
			logger.Errorf("[prome_push_queue_error][point: %+v] ", point)
		}
	}
}

func (pd *PromeDataSource) remoteWrite() {
	batch := pd.Section.Batch // 一次发送,最多batch条数据
	if batch <= 0 {
		batch = DefaultPopNum
	}
	for {
		items := pd.PushQueue.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		pbItems := make([]prompb.TimeSeries, count)
		for i := 0; i < count; i++ {
			pbItems[i] = items[i].(prompb.TimeSeries)
		}
		payload, err := pd.buildWriteRequest(pbItems)
		if err != nil {
			logger.Errorf("[prome_remote_write_error][pb_marshal_error][items: %+v][pb.err: %v]: ", items, err)
			continue
		}
		pd.processWrite(payload)

	}
}

func (pd *PromeDataSource) processWrite(payload []byte) {

	retry := pd.Section.MaxRetry

	for _, c := range pd.WriteTargets {
		newC := c
		go func(cc *HttpClient, payload []byte) {

			sendOk := false
			var err error
			for i := 0; i < retry; i++ {
				err := remoteWritePost(cc, payload)
				if err == nil {
					sendOk = true
					break
				}
				err, ok := err.(RecoverableError)

				if !ok {
					break
				}
				logger.Warningf("send prome fail: %v", err)
				time.Sleep(time.Millisecond * 100)
			}
			if !sendOk {
				logger.Warningf("send prome finally fail: %v", err)
			} else {
				logger.Infof("send to prome %s ok", cc.url.String())
			}
		}(newC, payload)
	}

}
