package prom

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

type WriterType struct {
	Opts   ClientOptions
	Client api.Client
}

func NewWriter(cli api.Client, opt ClientOptions) WriterType {
	writer := WriterType{
		Opts:   opt,
		Client: cli,
	}
	return writer
}

func (w WriterType) Write(items []prompb.TimeSeries, headers ...map[string]string) error {
	if len(items) == 0 {
		return nil
	}

	req := &prompb.WriteRequest{
		Timeseries: items,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		logger.Warningf("marshal prom data to proto got error: %v, data: %+v", err, items)
		return nil
	}

	if err := w.Post(snappy.Encode(nil, data), headers...); err != nil {
		logger.Warningf("%v post to %s got error: %v", w.Opts, w.Opts.Url, err)
		logger.Debug("example timeseries:", items[0].String())
	}
	return err
}

func (w WriterType) Post(req []byte, headers ...map[string]string) error {
	urls := strings.Split(w.Opts.Url, ",")
	var err error
	var httpReq *http.Request

	for _, url := range urls {
		httpReq, err = http.NewRequest("POST", url, bytes.NewReader(req))
		if err != nil {
			logger.Warningf("create remote write:%s request got error: %s", url, err.Error())
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

		if resp.StatusCode >= 400 {
			err = fmt.Errorf("push data with remote write:%s request got status code: %v, response body: %s", url, resp.StatusCode, string(body))
			logger.Warning(err)
			continue
		}

		break
	}

	return err
}
