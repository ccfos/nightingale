package writer

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"time"

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
}

func (w WriterType) writeRelabel(items []*prompb.TimeSeries) []*prompb.TimeSeries {
	ritems := make([]*prompb.TimeSeries, 0, len(items))
	for _, item := range items {
		lbls := Process(item.Labels, w.Opts.WriteRelabels...)
		if len(lbls) == 0 {
			continue
		}
		ritems = append(ritems, item)
	}
	return ritems
}

func (w WriterType) Write(index int, items []*prompb.TimeSeries, headers ...map[string]string) {
	if len(items) == 0 {
		return
	}

	items = w.writeRelabel(items)
	if len(items) == 0 {
		return
	}

	if w.ForceUseServerTS {
		ts := time.Now().UnixMilli()
		for i := 0; i < len(items); i++ {
			if len(items[i].Samples) == 0 {
				continue
			}
			items[i].Samples[0].Timestamp = ts
		}
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

	headerCount := len(w.Opts.Headers)
	if headerCount > 0 && headerCount%2 == 0 {
		for i := 0; i < len(w.Opts.Headers); i += 2 {
			httpReq.Header.Add(w.Opts.Headers[i], w.Opts.Headers[i+1])
			if w.Opts.Headers[i] == "Host" {
				httpReq.Host = w.Opts.Headers[i+1]
			}
		}
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
	pushgw   pconf.Pushgw
	backends map[string]WriterType
	queues   map[int]*SafeListLimited
}

func NewWriters(pushgwConfig pconf.Pushgw) *WritersType {
	writers := &WritersType{
		backends: make(map[string]WriterType),
		pushgw:   pushgwConfig,
	}

	writers.Init()
	return writers
}

func (ws *WritersType) Put(name string, writer WriterType) {
	ws.backends[name] = writer
}

func (ws *WritersType) PushSample(ident string, v interface{}) {
	hashkey := crc32.ChecksumIEEE([]byte(ident)) % uint32(ws.pushgw.WriterOpt.QueueCount)

	c, ok := ws.queues[int(hashkey)]
	if ok {
		succ := c.PushFront(v)
		if !succ {
			logger.Warningf("Write channel(%s) full, current channel size: %d", ident, c.Len())
		}
	}
}

func (ws *WritersType) StartConsumer(index int, ch *SafeListLimited) {
	for {
		series := ch.PopBack(ws.pushgw.WriterOpt.QueuePopSize)
		if len(series) == 0 {
			time.Sleep(time.Millisecond * 400)
			continue
		}

		for key := range ws.backends {
			go ws.backends[key].Write(index, series)
		}
	}
}

func (ws *WritersType) Init() error {
	opts := ws.pushgw.Writers
	ws.queues = make(map[int]*SafeListLimited)

	for i := 0; i < ws.pushgw.WriterOpt.QueueCount; i++ {
		ws.queues[i] = NewSafeListLimited(ws.pushgw.WriterOpt.QueueMaxSize)
		go ws.StartConsumer(i, ws.queues[i])
	}

	for i := 0; i < len(opts); i++ {
		tlsConf, err := opts[i].ClientConfig.TLSConfig()
		if err != nil {
			return err
		}

		trans := &http.Transport{
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
		}

		if tlsConf != nil {
			trans.TLSClientConfig = tlsConf
		}

		cli, err := api.NewClient(api.Config{
			Address:      opts[i].Url,
			RoundTripper: trans,
		})

		if err != nil {
			return err
		}

		writer := WriterType{
			Opts:             opts[i],
			Client:           cli,
			ForceUseServerTS: ws.pushgw.ForceUseServerTS,
		}

		ws.Put(opts[i].Url, writer)
	}

	return nil
}
