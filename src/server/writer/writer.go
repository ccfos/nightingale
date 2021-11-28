package writer

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

type Options struct {
	Name          string
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

func (w WriterType) Write(items []*prompb.TimeSeries) {
	req := &prompb.WriteRequest{
		Timeseries: items,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %v, data: %+v", err, items)
		return
	}

	if err := w.Post(snappy.Encode(nil, data)); err != nil {
		logger.Warningf("post to %s got error: %v", w.Opts.Url, err)
	}
}

func (w WriterType) Post(req []byte) error {
	httpReq, err := http.NewRequest("POST", w.Opts.Url, bytes.NewReader(req))
	if err != nil {
		logger.Warningf("create remote write request got error: %s", err.Error())
		return err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "n9e")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	if w.Opts.BasicAuthUser != "" {
		httpReq.SetBasicAuth(w.Opts.BasicAuthUser, w.Opts.BasicAuthPass)
	}

	resp, body, err := w.Client.Do(context.Background(), httpReq)
	if err != nil {
		logger.Warningf("push data with remote write request got error: %v, response body: %s", err, string(body))
		return err
	}

	if resp.StatusCode >= 400 {
		logger.Warningf("push data with remote write request got status code: %v, response body: %s", resp.StatusCode, string(body))
		return err
	}

	return nil
}

type WritersType struct {
	globalOpt GlobalOpt
	m         map[string]WriterType
	queue     *list.SafeListLimited
}

func (ws *WritersType) Put(name string, writer WriterType) {
	ws.m[name] = writer
}

func (ws *WritersType) PushQueue(vs []interface{}) bool {
	return ws.queue.PushFrontBatch(vs)
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

		Writers.Put(opts[i].Name, writer)
	}

	go Writers.Writes()

	return nil
}
