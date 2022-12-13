package writer

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"

	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

type WriterType struct {
	Opts   config.WriterOptions
	Client api.Client
}

func (w WriterType) writeRelabel(items []*prompb.TimeSeries) []*prompb.TimeSeries {
	ritems := make([]*prompb.TimeSeries, 0, len(items))
	for _, item := range items {
		lbls := models.Process(item.Labels, w.Opts.WriteRelabels...)
		if len(lbls) == 0 {
			continue
		}
		ritems = append(ritems, item)
	}
	return ritems
}

func (w WriterType) Write(cluster string, index int, items []*prompb.TimeSeries, headers ...map[string]string) {
	if len(items) == 0 {
		return
	}

	items = w.writeRelabel(items)
	if len(items) == 0 {
		return
	}

	start := time.Now()
	defer func() {
		if cluster != "" {
			promstat.ForwardDuration.WithLabelValues(cluster, fmt.Sprint(index)).Observe(time.Since(start).Seconds())
		}
	}()

	if config.C.ForceUseServerTS {
		ts := start.UnixMilli()
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
	globalOpt config.WriterGlobalOpt
	backends  map[string]WriterType
	queues    map[string]map[int]*SafeListLimited
}

func (ws *WritersType) Put(name string, writer WriterType) {
	ws.backends[name] = writer
}

func (ws *WritersType) PushSample(ident string, v interface{}, clusters ...string) {
	hashkey := crc32.ChecksumIEEE([]byte(ident)) % uint32(ws.globalOpt.QueueCount)

	cluster := config.C.ClusterName
	if len(clusters) > 0 {
		cluster = clusters[0]
	}

	if _, ok := ws.queues[cluster]; !ok {
		// 待写入的集群不存在
		logger.Warningf("Write cluster:%s not found, v:%+v", cluster, v)
		return
	}

	c, ok := ws.queues[cluster][int(hashkey)]
	if ok {
		succ := c.PushFront(v)
		if !succ {
			logger.Warningf("Write channel(%s) full, current channel size: %d", ident, c.Len())
		}
	}
}

func (ws *WritersType) StartConsumer(index int, ch *SafeListLimited, clusterName string) {
	for {
		series := ch.PopBack(ws.globalOpt.QueuePopSize)
		if len(series) == 0 {
			time.Sleep(time.Millisecond * 400)
			continue
		}

		for key := range ws.backends {
			if ws.backends[key].Opts.ClusterName != clusterName {
				continue
			}
			go ws.backends[key].Write(clusterName, index, series)
		}
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
	Writers.queues = make(map[string]map[int]*SafeListLimited)
	for _, opt := range opts {
		if _, ok := Writers.queues[opt.ClusterName]; !ok {
			Writers.queues[opt.ClusterName] = make(map[int]*SafeListLimited)
			for i := 0; i < globalOpt.QueueCount; i++ {
				Writers.queues[opt.ClusterName][i] = NewSafeListLimited(Writers.globalOpt.QueueMaxSize)
				go Writers.StartConsumer(i, Writers.queues[opt.ClusterName][i], opt.ClusterName)
			}
		}
	}

	go reportChanSize()

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

func reportChanSize() {
	clusterName := config.C.ClusterName
	if clusterName == "" {
		return
	}

	for {
		time.Sleep(time.Second * 3)
		for cluster, m := range Writers.queues {
			for i, c := range m {
				size := c.Len()
				promstat.GaugeSampleQueueSize.WithLabelValues(cluster, fmt.Sprint(i)).Set(float64(size))
			}
		}
	}
}
