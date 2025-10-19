package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/ccfos/nightingale/v6/pkg/fasttime"
	"github.com/ccfos/nightingale/v6/pushgw/kafka"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

type WriterType struct {
	Opts             pconf.WriterOptions
	ForceUseServerTS bool
	Client           api.Client
	RetryCount       int
	RetryInterval    int64 // 单位秒
}

func beforeWrite(key string, items []prompb.TimeSeries, forceUseServerTS bool, encodeType string) ([]byte, error) {
	pstat.CounterWirteTotal.WithLabelValues(key).Add(float64(len(items)))

	if forceUseServerTS {
		ts := int64(fasttime.UnixTimestamp()) * 1000
		for i := 0; i < len(items); i++ {
			if len(items[i].Samples) == 0 {
				continue
			}
			items[i].Samples[0].Timestamp = ts
		}
	}

	if encodeType == "proto" {
		req := &prompb.WriteRequest{
			Timeseries: items,
		}

		return proto.Marshal(req)
	}
	// 如果是 json 格式，将 NaN 值的数据丢弃掉
	return json.Marshal(filterNaNSamples(items))
}

func filterNaNSamples(items []prompb.TimeSeries) []prompb.TimeSeries {
	// 早期检查：如果没有NaN值，直接返回原始数据
	hasNaN := false
	for i := range items {
		for j := range items[i].Samples {
			if math.IsNaN(items[i].Samples[j].Value) {
				hasNaN = true
				break
			}
		}
		if hasNaN {
			break
		}
	}

	if !hasNaN {
		return items
	}

	// 有NaN值时进行过滤，原地修改以减少内存分配
	for i := range items {
		samples := items[i].Samples
		validCount := 0

		// 原地过滤 samples，避免额外的内存分配
		for j := range samples {
			if !math.IsNaN(samples[j].Value) {
				if validCount != j {
					samples[validCount] = samples[j]
				}
				validCount++
			}
		}

		// 保留所有时间序列，即使没有有效样本（此时Samples为空）
		items[i].Samples = samples[:validCount]
	}

	return items
}

func (w WriterType) Write(key string, items []prompb.TimeSeries, headers ...map[string]string) {
	if len(items) == 0 {
		return
	}

	items = Relabel(items, w.Opts.WriteRelabels)
	if len(items) == 0 {
		return
	}

	start := time.Now()
	defer func() {
		pstat.ForwardDuration.WithLabelValues(key).Observe(time.Since(start).Seconds())
	}()

	data, err := beforeWrite(key, items, w.ForceUseServerTS, "proto")
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %v, data: %+v", err, items)
		return
	}

	for i := 0; i < w.RetryCount; i++ {
		err := w.Post(snappy.Encode(nil, data), headers...)
		if err == nil {
			break
		}

		pstat.CounterWirteErrorTotal.WithLabelValues(key).Add(float64(len(items)))
		logger.Warningf("post to %s got error: %v in %d times", w.Opts.Url, err, i)

		if i == 0 {
			logger.Warning("example timeseries:", items[0].String())
		}

		time.Sleep(time.Duration(w.RetryInterval) * time.Second)
	}
}

func (w WriterType) Post(req []byte, headers ...map[string]string) error {
	urls := strings.Split(w.Opts.Url, ",")
	var err error
	var newRequestErr error
	var httpReq *http.Request
	for _, url := range urls {
		httpReq, newRequestErr = http.NewRequest("POST", url, bytes.NewReader(req))
		if newRequestErr != nil {
			logger.Warningf("create remote write:%s request got error: %s", url, newRequestErr.Error())
			continue
		}

		httpReq.Header.Add("Content-Encoding", "snappy")
		httpReq.Header.Set("Content-Type", "application/x-protobuf")
		httpReq.Header.Set("User-Agent", "n9e")
		httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

		if len(headers) > 0 {
			for k, v := range headers[0] {
				httpReq.Header.Set(k, v)
			}
		}

		if w.Opts.BasicAuthUser != "" {
			httpReq.SetBasicAuth(w.Opts.BasicAuthUser, w.Opts.BasicAuthPass)
		}

		headerCount := len(w.Opts.Headers)
		if headerCount > 0 && headerCount%2 == 0 {
			for i := 0; i < len(w.Opts.Headers); i += 2 {
				httpReq.Header.Add(w.Opts.Headers[i], w.Opts.Headers[i+1])
				if w.Opts.Headers[i] == "Host" {
					httpReq.Host = w.Opts.Headers[i+1]
				}
			}
		}

		resp, body, e := w.Client.Do(context.Background(), httpReq)
		if e != nil {
			logger.Warningf("push data with remote write:%s request got error: %v, response body: %s", url, e, string(body))
			err = e
			continue
		}

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			// 解码并解析 req 以便打印指标信息
			decoded, decodeErr := snappy.Decode(nil, req)
			metricsInfo := "failed to decode request"
			if decodeErr == nil {
				var writeReq prompb.WriteRequest
				if unmarshalErr := proto.Unmarshal(decoded, &writeReq); unmarshalErr == nil {
					metricsInfo = fmt.Sprintf("timeseries count: %d", len(writeReq.Timeseries))
					logger.Warningf("push data with remote write:%s request got status code: %v, response body: %s, %s", url, resp.StatusCode, string(body), metricsInfo)
					// 只打印前几条样本，避免日志泛滥
					sampleCount := 5
					if sampleCount > len(writeReq.Timeseries) {
						sampleCount = len(writeReq.Timeseries)
					}
					for i := 0; i < sampleCount; i++ {
						logger.Warningf("push data with remote write:%s timeseries: [%d] %s", url, i, writeReq.Timeseries[i].String())
					}
				} else {
					metricsInfo = fmt.Sprintf("failed to unmarshal: %v", unmarshalErr)
					logger.Warningf("push data with remote write:%s request got status code: %v, response body: %s, metrics: %s", url, resp.StatusCode, string(body), metricsInfo)
				}
			} else {
				metricsInfo = fmt.Sprintf("failed to decode: %v", decodeErr)
				logger.Warningf("push data with remote write:%s request got status code: %v, response body: %s, metrics: %s", url, resp.StatusCode, string(body), metricsInfo)
			}
			continue
		}

		if resp.StatusCode >= 500 {
			err = fmt.Errorf("push data with remote write:%s request got status code: %v, response body: %s", url, resp.StatusCode, string(body))
			logger.Warning(err)
			continue
		}

		err = nil
		break
	}

	return err
}

type WritersType struct {
	pushgw          pconf.Pushgw
	backends        map[string]Writer
	queues          map[string]*IdentQueue
	AllQueueLen     atomic.Value
	PushConcurrency atomic.Int64
	sync.RWMutex
}

type IdentQueue struct {
	list    *SafeListLimited
	closeCh chan struct{}
	ts      int64
}

func (ws *WritersType) ReportQueueStats(queueid string, identQueue *IdentQueue) (interface{}, bool) {
	for {
		time.Sleep(15 * time.Second)
		count := identQueue.list.Len()
		pstat.GaugeSampleQueueSize.WithLabelValues(queueid).Set(float64(count))
	}
}

func (ws *WritersType) SetAllQueueLen() {
	for {
		curMetricLen := 0
		ws.RLock()
		for _, q := range ws.queues {
			curMetricLen += q.list.Len()
		}
		ws.RUnlock()
		ws.AllQueueLen.Store(int64(curMetricLen))
		time.Sleep(time.Duration(ws.pushgw.WriterOpt.AllQueueMaxSizeInterval) * time.Millisecond)
	}
}

func NewWriters(pushgwConfig pconf.Pushgw) *WritersType {
	writers := &WritersType{
		backends:    make(map[string]Writer),
		queues:      make(map[string]*IdentQueue),
		pushgw:      pushgwConfig,
		AllQueueLen: atomic.Value{},
	}

	writers.Init()

	go writers.SetAllQueueLen()
	go writers.CleanExpQueue()
	return writers
}

func (ws *WritersType) Put(name string, writer Writer) {
	ws.backends[name] = writer
}

func (ws *WritersType) isCriticalBackend(key string) bool {
	backend, exists := ws.backends[key]
	if !exists {
		return false
	}

	// 使用类型断言判断
	switch backend.(type) {
	case WriterType:
		if backend.(WriterType).Opts.AsyncWrite {
			return false
		}

		// HTTP Writer 作为关键后端
		return true
	case KafkaWriterType:
		// Kafka Writer 作为非关键后端
		return false
	default:
		// 未知类型，保守起见作为关键后端
		logger.Warningf("Unknown backend type: %T, treating as critical", backend)
		return true
	}
}

func (ws *WritersType) CleanExpQueue() {
	for {
		ws.Lock()
		for ident := range ws.queues {
			identQueue := ws.queues[ident]
			if identQueue == nil {
				delete(ws.queues, ident)
				logger.Warningf("Write channel(%s) not found", ident)
				continue
			}

			if time.Now().Unix()-identQueue.ts > 3600 {
				close(identQueue.closeCh)
				delete(ws.queues, ident)
			}
		}
		ws.Unlock()
		time.Sleep(time.Second * 600)
	}
}

func (ws *WritersType) PushSample(queueid string, v interface{}) error {
	ws.RLock()
	queue := ws.queues[queueid]
	ws.RUnlock()
	if queue == nil {
		queue = &IdentQueue{
			list:    NewSafeListLimited(ws.pushgw.WriterOpt.QueueMaxSize),
			closeCh: make(chan struct{}),
			ts:      time.Now().Unix(),
		}

		ws.Lock()
		ws.queues[queueid] = queue
		ws.Unlock()

		go ws.ReportQueueStats(queueid, queue)
		go ws.StartConsumer(queue)
	}

	queue.ts = time.Now().Unix()

	succ := queue.list.PushFront(v)
	if !succ {
		logger.Warningf("Write channel(%s) full, current channel size: %d, item: %+v", queueid, queue.list.Len(), v)
		pstat.CounterPushQueueErrorTotal.WithLabelValues(queueid).Inc()
	}

	return nil
}

type Writer interface {
	Write(string, []prompb.TimeSeries, ...map[string]string)
}

func (ws *WritersType) StartConsumer(identQueue *IdentQueue) {
	for {
		select {
		case <-identQueue.closeCh:
			logger.Infof("write queue:%v closed", identQueue)
			return
		default:
			series := identQueue.list.PopBack(ws.pushgw.WriterOpt.QueuePopSize)
			if len(series) == 0 {
				time.Sleep(time.Millisecond * 400)
				continue
			}
			for key := range ws.backends {

				if ws.isCriticalBackend(key) {
					ws.backends[key].Write(key, series)
				} else {
					// 像 kafka 这种 writer 使用异步写入，防止因为写入太慢影响主流程
					ws.writeToNonCriticalBackend(key, series)
				}
			}
		}
	}
}

func (ws *WritersType) writeToNonCriticalBackend(key string, series []prompb.TimeSeries) {
	// 原子性地检查并增加并发数
	currentConcurrency := ws.PushConcurrency.Add(1)

	if currentConcurrency > int64(ws.pushgw.PushConcurrency) {
		// 超过限制，立即减少计数并丢弃
		ws.PushConcurrency.Add(-1)
		logger.Warningf("push concurrency limit exceeded, current: %d, limit: %d, dropping %d series for backend: %s",
			currentConcurrency-1, ws.pushgw.PushConcurrency, len(series), key)
		pstat.CounterWirteErrorTotal.WithLabelValues(key).Add(float64(len(series)))
		return
	}

	// 深拷贝数据，确保并发安全
	seriesCopy := ws.deepCopySeries(series)

	// 启动goroutine处理
	go func(backendKey string, data []prompb.TimeSeries) {
		defer func() {
			ws.PushConcurrency.Add(-1)
			if r := recover(); r != nil {
				logger.Errorf("panic in non-critical backend %s: %v", backendKey, r)
			}
		}()

		ws.backends[backendKey].Write(backendKey, data)
	}(key, seriesCopy)
}

// 完整的深拷贝方法
func (ws *WritersType) deepCopySeries(series []prompb.TimeSeries) []prompb.TimeSeries {
	seriesCopy := make([]prompb.TimeSeries, len(series))

	for i := range series {
		seriesCopy[i] = series[i]

		if len(series[i].Samples) > 0 {
			samples := make([]prompb.Sample, len(series[i].Samples))
			copy(samples, series[i].Samples)
			seriesCopy[i].Samples = samples
		}
	}

	return seriesCopy
}

func (ws *WritersType) Init() error {
	ws.AllQueueLen.Store(int64(0))

	if err := ws.initWriters(); err != nil {
		return err
	}

	return ws.initKafkaWriters()
}

func (ws *WritersType) initWriters() error {
	opts := ws.pushgw.Writers

	for i := range opts {
		cli, err := api.NewClient(api.Config{
			Address:      opts[i].Url,
			RoundTripper: opts[i].HTTPTransport,
		})

		if err != nil {
			return err
		}

		writer := WriterType{
			Opts:             opts[i],
			Client:           cli,
			ForceUseServerTS: ws.pushgw.ForceUseServerTS,
			RetryCount:       ws.pushgw.WriterOpt.RetryCount,
			RetryInterval:    ws.pushgw.WriterOpt.RetryInterval,
		}

		ws.Put(opts[i].Url, writer)
	}

	return nil
}

func initKakfaSASL(cfg *sarama.Config, opt pconf.KafkaWriterOptions) {
	if opt.SASL != nil && opt.SASL.Enable {
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = opt.SASL.User
		cfg.Net.SASL.Password = opt.SASL.Password
		cfg.Net.SASL.Mechanism = sarama.SASLMechanism(opt.SASL.Mechanism)
		cfg.Net.SASL.Version = opt.SASL.Version
		cfg.Net.SASL.Handshake = opt.SASL.Handshake
		cfg.Net.SASL.AuthIdentity = opt.SASL.AuthIdentity
	}
}

func (ws *WritersType) initKafkaWriters() error {
	opts := ws.pushgw.KafkaWriters

	for i := 0; i < len(opts); i++ {
		cfg := sarama.NewConfig()
		initKakfaSASL(cfg, opts[i])
		if opts[i].Timeout != 0 {
			cfg.Producer.Timeout = time.Duration(opts[i].Timeout) * time.Second
		}
		if opts[i].Version != "" {
			kafkaVersion, err := sarama.ParseKafkaVersion(opts[i].Version)
			if err != nil {
				logger.Warningf("parse kafka version got error: %v", err)
			} else {
				cfg.Version = kafkaVersion
			}
		}

		if opts[i].Typ == "" {
			opts[i].Typ = kafka.AsyncProducer
		}

		producer, err := kafka.New(opts[i].Typ, opts[i].Brokers, cfg)
		if err != nil {
			logger.Warningf("new kafka producer got error: %v", err)
			return err
		}

		writer := KafkaWriterType{
			Opts:             opts[i],
			ForceUseServerTS: ws.pushgw.ForceUseServerTS,
			Client:           producer,
			RetryCount:       ws.pushgw.WriterOpt.RetryCount,
			RetryInterval:    ws.pushgw.WriterOpt.RetryInterval,
		}
		ws.Put(fmt.Sprintf("%v_%s", opts[i].Brokers, opts[i].Topic), writer)
	}

	return nil
}
