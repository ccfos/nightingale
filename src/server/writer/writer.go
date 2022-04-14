package writer

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	cmap "github.com/orcaman/concurrent-map"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
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

func (w WriterType) Write(items []*prompb.TimeSeries, headers ...map[string]string) {
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

	if err := w.Post(snappy.Encode(nil, data), headers...); err != nil {
		logger.Warningf("post to %s got error: %v", w.Opts.Url, err)
		logger.Warning("example timeseries:", items[0].String())
	}
}

func (w WriterType) Post(req []byte, headers ...map[string]string) error {
	httpReq, err := http.NewRequest("POST", w.Opts.Url, bytes.NewReader(req))
	if err != nil {
		logger.Warningf("create remote write request got error: %s", err.Error())
		return err
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
	globalOpt GlobalOpt
	backends  map[string]WriterType
	chans     cmap.ConcurrentMap
	sync.RWMutex
}

func (ws *WritersType) Put(name string, writer WriterType) {
	ws.backends[name] = writer
}

// PushSample Push one sample to chan, hash by ident
// @Author: quzhihao
func (ws *WritersType) PushSample(ident string, v interface{}) {
	if !ws.chans.Has(ident) {
		ws.Lock()
		// important: check twice
		if !ws.chans.Has(ident) {
			c := make(chan *prompb.TimeSeries, Writers.globalOpt.QueueMaxSize)
			ws.chans.Set(ident, c)
			go ws.StartConsumer(ident, c)
		}
		ws.Unlock()
	}

	c, ok := ws.chans.Get(ident)
	if ok {
		ch := c.(chan *prompb.TimeSeries)
		select {
		case ch <- v.(*prompb.TimeSeries):
		default:
			logger.Warningf("Write channel(%s) full, current channel size: %d", ident, len(ch))
		}
	}
}

// StartConsumer every ident channel has a consumer, start it
// @Author: quzhihao
func (ws *WritersType) StartConsumer(ident string, ch chan *prompb.TimeSeries) {
	var (
		batch        = ws.globalOpt.QueuePopSize
		max          = ws.globalOpt.QueueMaxSize
		batchCounter int
		closeCounter int
		series       = make([]*prompb.TimeSeries, 0, batch)
	)

	logger.Infof("Starting channel(%s) consumer, max size:%d, batch:%d", ident, max, batch)

	for {
		select {
		case item := <-ch:
			// has data, no need to close
			closeCounter = 0
			series = append(series, item)

			batchCounter++
			if batchCounter >= ws.globalOpt.QueuePopSize {
				ws.post(ident, series)

				// reset
				batchCounter = 0
				series = make([]*prompb.TimeSeries, 0, batch)
			}
		case <-time.After(time.Second):
			if len(series) > 0 {
				// has data, no need to close
				closeCounter = 0

				ws.post(ident, series)

				// reset
				batchCounter = 0
				series = make([]*prompb.TimeSeries, 0, batch)
			} else {
				closeCounter++
			}

			if closeCounter > 3600 {
				logger.Infof("Closing channel(%s) reason: no data for an hour", ident)

				ws.Lock()
				close(ch)
				ws.chans.Remove(ident)
				ws.Unlock()

				logger.Infof("Closed channel(%s) reason: no data for an hour", ident)

				return
			}
		}
	}
}

// post post series to TSDB
// @Author: quzhihao
func (ws *WritersType) post(ident string, series []*prompb.TimeSeries) {
	wg := sync.WaitGroup{}
	wg.Add(len(ws.backends))

	// maybe as backend hashstring
	headers := map[string]string{"ident": ident}
	for key := range ws.backends {
		go func(key string) {
			defer wg.Done()
			ws.backends[key].Write(series, headers)
		}(key)
	}

	wg.Wait()
}

func NewWriters() WritersType {
	return WritersType{
		backends: make(map[string]WriterType),
	}
}

var Writers = NewWriters()

func Init(opts []Options, globalOpt GlobalOpt) error {
	Writers.globalOpt = globalOpt
	Writers.chans = cmap.New()

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

	return nil
}
