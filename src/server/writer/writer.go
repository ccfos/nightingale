package writer

import (
	"bytes"
	"context"
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

type Options struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type GlobalOpt struct {
	QueueMaxSize  int
	QueuePopSize  int
	SleepInterval int64
}

type WriterType struct {
	Opts   Options
	Client api.Client
}

var lock = sync.RWMutex{}

func (w WriterType) Write(items []*prompb.TimeSeries) {
	if len(items) == 0 {
		return
	}

	req := &prompb.WriteRequest{
		Timeseries: items,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %v, data: %+v", err, items)
		return
	}

	if err := w.Post(snappy.Encode(nil, data), nil); err != nil {
		logger.Warningf("post to %s got error: %v", w.Opts.Url, err)
		logger.Warning("example timeseries:", items[0].String())
	}
}

func (w WriterType) WriteWithHeader(items []*prompb.TimeSeries, headerMap map[string]string) {
	if len(items) == 0 {
		return
	}

	req := &prompb.WriteRequest{
		Timeseries: items,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %v, data: %+v", err, items)
		return
	}

	if err := w.Post(snappy.Encode(nil, data), headerMap); err != nil {
		logger.Warningf("post to %s got error: %v", w.Opts.Url, err)
		logger.Warning("example timeseries:", items[0].String())
	}
}

func (w WriterType) Post(req []byte, headerMap map[string]string) error {
	httpReq, err := http.NewRequest("POST", w.Opts.Url, bytes.NewReader(req))
	if err != nil {
		logger.Warningf("create remote write request got error: %s", err.Error())
		return err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "n9e")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	if headerMap != nil {
		for k, v := range headerMap {
			httpReq.Header.Set(k, v)
		}
	}

	if w.Opts.BasicAuthUser != "" {
		httpReq.SetBasicAuth(w.Opts.BasicAuthUser, w.Opts.BasicAuthPass)
	}

	resp, body, err := w.Client.Do(context.Background(), httpReq)
	if err != nil {
		logger.Warningf("push data with remote write request got error: %v, response body: %s", err, string(body))
		return err
	}

	if resp.StatusCode >= 400 {
		err = fmt.Errorf("push data with remote write request got status code: %v, response body: %s", resp.StatusCode, string(body))
		return err
	}

	return nil
}

type WritersType struct {
	globalOpt    GlobalOpt
	m            map[string]WriterType
	queue        *list.SafeListLimited
	IdentChanMap cmap.ConcurrentMap
}

func (ws *WritersType) Put(name string, writer WriterType) {
	ws.m[name] = writer
}

func (ws *WritersType) PushQueue(vs []interface{}) bool {
	return ws.queue.PushFrontBatch(vs)
}

//
// PushIdentChan 放入chan， 以ident分发
// @Author: quzhihao
// @Description:
// @receiver ws
// @param ident
// @param vs
//
func (ws *WritersType) PushIdentChan(ident string, vs interface{}) {
	if !ws.IdentChanMap.Has(ident) {
		lock.Lock()
		if !ws.IdentChanMap.Has(ident) {
			c := make(chan *prompb.TimeSeries, Writers.globalOpt.QueueMaxSize)
			ws.IdentChanMap.Set(ident, c)
			go func() {
				ws.InitIdentChanWorker(ident, c)
			}()
		}
		lock.Unlock()
	}
	// 往chan扔会导致内存不断增大，如果写入阻塞了，需要提示
	c, ok := ws.IdentChanMap.Get(ident)
	ch := c.(chan *prompb.TimeSeries)
	if ok {
		select {
		case ch <- vs.(*prompb.TimeSeries):
		case <-time.After(time.Duration(200) * time.Millisecond):
			logger.Warningf("[%s] Write IdentChanMap Full, DropSize: %d", ident, len(ch))
		}
	}
}

//
// InitIdentChanWorker 初始化ident消费者
// @Author: quzhihao
// @Description:
// @receiver ws
// @param ident
// @param data
//
func (ws *WritersType) InitIdentChanWorker(ident string, data chan *prompb.TimeSeries) {
	popCounter := 0
	batch := ws.globalOpt.QueuePopSize
	if batch <= 0 {
		batch = 1000
	}
	logger.Infof("[%s] Start Ident Chan Worker, MaxSize:%d, batchSize:%d", ident, ws.globalOpt.QueueMaxSize, batch)
	series := make([]*prompb.TimeSeries, 0, batch)
	closePrepareCounter := 0
	for {
		select {
		case item := <-data:
			closePrepareCounter = 0
			series = append(series, item)
			popCounter++
			if popCounter >= ws.globalOpt.QueuePopSize {
				popCounter = 0
				// 发送到prometheus
				ws.postPrometheus(ident, series)
				series = make([]*prompb.TimeSeries, 0, batch)
			}
		case <-time.After(10 * time.Second):
			// 10秒清空一下，如果有数据的话
			if len(series) > 0 {
				ws.postPrometheus(ident, series)
				series = make([]*prompb.TimeSeries, 0, batch)
				closePrepareCounter = 0
			} else {
				closePrepareCounter++
			}
			// 一小时没数据，就关闭chan
			if closePrepareCounter > 6*60 {
				logger.Infof("[%s] Ident Chan Closing. Reason: No Data For An Hour.", ident)
				lock.Lock()
				close(data)
				// 移除
				ws.IdentChanMap.Remove(ident)
				lock.Unlock()
				logger.Infof("[%s] Ident Chan Closed Success.", ident)
				return
			}
		}
	}
}

//
// postPrometheus 发送数据至prometheus
// @Author: quzhihao
// @Description:
// @receiver ws
// @param ident
// @param series
//
func (ws *WritersType) postPrometheus(ident string, series []*prompb.TimeSeries) {
	// 发送至prom
	wg := sync.WaitGroup{}
	wg.Add(len(ws.m))
	headerMap := make(map[string]string, 1)
	headerMap["ident"] = ident
	for key := range ws.m {
		go func(key string) {
			defer wg.Done()
			ws.m[key].WriteWithHeader(series, headerMap)
		}(key)
	}
	wg.Wait()
}

func (ws *WritersType) Writes() {
	batch := ws.globalOpt.QueuePopSize
	if batch <= 0 {
		batch = 2000
	}

	duration := time.Duration(ws.globalOpt.SleepInterval) * time.Millisecond

	for {
		items := ws.queue.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(duration)
			continue
		}

		series := make([]*prompb.TimeSeries, 0, count)
		for i := 0; i < count; i++ {
			item, ok := items[i].(*prompb.TimeSeries)
			if !ok {
				// in theory, it can be converted successfully
				continue
			}
			series = append(series, item)
		}

		if len(series) == 0 {
			continue
		}

		for key := range ws.m {
			go ws.m[key].Write(series)
		}
	}
}

func NewWriters() WritersType {
	return WritersType{
		m: make(map[string]WriterType),
	}
}

var Writers = NewWriters()

func Init(opts []Options, globalOpt GlobalOpt) error {
	Writers.globalOpt = globalOpt
	Writers.queue = list.NewSafeListLimited(globalOpt.QueueMaxSize)

	for i := 0; i < len(opts); i++ {
		cli, err := api.NewClient(api.Config{
			Address: opts[i].Url,
			RoundTripper: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(opts[i].DialTimeout) * time.Millisecond,
					KeepAlive: time.Duration(opts[i].KeepAlive) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(opts[i].Timeout) * time.Millisecond,
				TLSHandshakeTimeout:   time.Duration(opts[i].TLSHandshakeTimeout) * time.Millisecond,
				ExpectContinueTimeout: time.Duration(opts[i].ExpectContinueTimeout) * time.Millisecond,
				MaxConnsPerHost:       opts[i].MaxConnsPerHost,
				MaxIdleConns:          opts[i].MaxIdleConns,
				MaxIdleConnsPerHost:   opts[i].MaxIdleConnsPerHost,
				IdleConnTimeout:       time.Duration(opts[i].IdleConnTimeout) * time.Millisecond,
			},
		})

		if err != nil {
			return err
		}

		writer := WriterType{
			Opts:   opts[i],
			Client: cli,
		}

		Writers.Put(opts[i].Url, writer)
	}

	go Writers.Writes()

	return nil
}
