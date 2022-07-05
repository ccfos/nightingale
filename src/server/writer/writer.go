package writer

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

type WriterType struct {
	Opts   config.WriterOptions
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
	globalOpt config.WriterGlobalOpt
	backends  map[string]WriterType
	chans     map[int]chan *prompb.TimeSeries
}

func (ws *WritersType) Put(name string, writer WriterType) {
	ws.backends[name] = writer
}

// PushSample Push one sample to chan, hash by ident
// @Author: quzhihao
func (ws *WritersType) PushSample(ident string, v interface{}) {
	hashkey := crc32.ChecksumIEEE([]byte(ident)) % uint32(ws.globalOpt.QueueCount)

	c, ok := ws.chans[int(hashkey)]
	if ok {
		select {
		case c <- v.(*prompb.TimeSeries):
		default:
			logger.Warningf("Write channel(%s) full, current channel size: %d", ident, len(c))
		}
	}
}

// StartConsumer every ident channel has a consumer, start it
// @Author: quzhihao
func (ws *WritersType) StartConsumer(index int, ch chan *prompb.TimeSeries) {
	var (
		batch        = ws.globalOpt.QueuePopSize
		series       = make([]*prompb.TimeSeries, 0, batch)
		batchCounter int
	)

	for {
		select {
		case item := <-ch:
			// has data, no need to close
			series = append(series, item)

			batchCounter++
			if batchCounter >= ws.globalOpt.QueuePopSize {
				ws.post(index, series)

				// reset
				batchCounter = 0
				series = make([]*prompb.TimeSeries, 0, batch)
			}
		case <-time.After(time.Second):
			if len(series) > 0 {
				ws.post(index, series)

				// reset
				batchCounter = 0
				series = make([]*prompb.TimeSeries, 0, batch)
			}
		}
	}
}

// post post series to TSDB
// @Author: quzhihao
func (ws *WritersType) post(index int, series []*prompb.TimeSeries) {
	header := map[string]string{"hash": fmt.Sprintf("%s-%d", config.C.Heartbeat.Endpoint, index)}
	if len(ws.backends) == 1 {
		for key := range ws.backends {
			ws.backends[key].Write(series, header)
		}
		return
	}

	if len(ws.backends) > 1 {
		wg := new(sync.WaitGroup)
		for key := range ws.backends {
			wg.Add(1)
			go func(wg *sync.WaitGroup, backend WriterType, items []*prompb.TimeSeries, header map[string]string) {
				defer wg.Done()
				backend.Write(series, header)
			}(wg, ws.backends[key], series, header)
		}
		wg.Wait()
	}
}

func NewWriters() WritersType {
	return WritersType{
		backends: make(map[string]WriterType),
	}
}

var Writers = NewWriters()

func Init(opts []config.WriterOptions, globalOpt config.WriterGlobalOpt) error {
	Writers.globalOpt = globalOpt
	Writers.chans = make(map[int]chan *prompb.TimeSeries)

	// init channels
	for i := 0; i < globalOpt.QueueCount; i++ {
		Writers.chans[i] = make(chan *prompb.TimeSeries, Writers.globalOpt.QueueMaxSize)
		go Writers.StartConsumer(i, Writers.chans[i])
	}

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
