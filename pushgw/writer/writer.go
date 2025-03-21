package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/ccfos/nightingale/v6/pkg/fasttime"
	"github.com/ccfos/nightingale/v6/pushgw/kafka"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"

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
	CounterWirteTotal.WithLabelValues(key).Add(float64(len(items)))

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

	return json.Marshal(items)
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
		ForwardDuration.WithLabelValues(key).Observe(time.Since(start).Seconds())
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

		CounterWirteErrorTotal.WithLabelValues(key).Add(float64(len(items)))
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
			logger.Warningf("push data with remote write:%s request got status code: %v, response body: %s", url, resp.StatusCode, string(body))
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
	pushgw      pconf.Pushgw
	backends    map[string]Writer
	queues      map[string]*IdentQueue
	AllQueueLen atomic.Value
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
		GaugeSampleQueueSize.WithLabelValues(queueid).Set(float64(count))
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
		CounterPushQueueErrorTotal.WithLabelValues(queueid).Inc()
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
				ws.backends[key].Write(key, series)
			}
		}
	}
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
