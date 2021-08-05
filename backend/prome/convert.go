package backend

import (
	"bufio"
	"bytes"
	"context"

	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/vos"
)

var MetricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

type sample struct {
	labels labels.Labels
	t      int64
	v      float64
}

func labelsToLabelsProto(labels labels.Labels, buf []prompb.Label) []prompb.Label {
	result := buf[:0]
	if cap(buf) < len(labels) {
		result = make([]prompb.Label, 0, len(labels))
	}
	for _, l := range labels {
		result = append(result, prompb.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	return result
}

func (pd *PromeDataSource) convertOne(item *vos.MetricPoint) (prompb.TimeSeries, error) {
	pt := prompb.TimeSeries{}
	pt.Samples = []prompb.Sample{{}}
	s := sample{}
	s.t = item.Time
	s.v = item.Value
	// name
	if !MetricNameRE.MatchString(item.Metric) {
		return pt, errors.New("invalid metrics name")
	}
	nameLs := labels.Label{
		Name:  LABEL_NAME,
		Value: item.Metric,
	}
	s.labels = append(s.labels, nameLs)
	if item.Ident != "" {
		identLs := labels.Label{
			Name:  LABEL_IDENT,
			Value: item.Ident,
		}
		s.labels = append(s.labels, identLs)
	}

	for k, v := range item.TagsMap {
		if model.LabelNameRE.MatchString(k) {
			ls := labels.Label{
				Name:  k,
				Value: v,
			}
			s.labels = append(s.labels, ls)

		}

	}

	pt.Labels = labelsToLabelsProto(s.labels, pt.Labels)
	// 时间赋值问题,使用毫秒时间戳
	tsMs := time.Unix(s.t, 0).UnixNano() / 1e6
	pt.Samples[0].Timestamp = tsMs
	pt.Samples[0].Value = s.v
	return pt, nil
}

type RecoverableError struct {
	error
}

func remoteWritePost(c *HttpClient, req []byte) error {
	httpReq, err := http.NewRequest("POST", c.url.String(), bytes.NewReader(req))
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return err
	}

	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", "n9e-v5")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	httpReq = httpReq.WithContext(ctx)

	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		var ht *nethttp.Tracer
		httpReq, ht = nethttp.TraceRequest(
			parentSpan.Tracer(),
			httpReq,
			nethttp.OperationName("Remote Store"),
			nethttp.ClientTrace(false),
		)
		defer ht.Finish()
	}

	httpResp, err := c.Client.Do(httpReq)
	if err != nil {
		// Errors from Client.Do are from (for example) network errors, so are
		// recoverable.
		return RecoverableError{err}
	}
	defer func() {
		io.Copy(ioutil.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	if httpResp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(httpResp.Body, 512))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}

		if httpResp.StatusCode == 400 {
			//400的错误是客户端的问题，不返回给上层，输出到debug日志中
			logger.Debugf("server returned HTTP status %s: %s req:%v", httpResp.Status, line, getSamples(req))
		} else {
			err = errors.Errorf("server returned HTTP status %s: %s", httpResp.Status, line)
		}
	}

	if httpResp.StatusCode/100 == 5 {
		return RecoverableError{err}
	}
	return err
}

func (pd *PromeDataSource) buildWriteRequest(samples []prompb.TimeSeries) ([]byte, error) {

	req := &prompb.WriteRequest{
		Timeseries: samples,
		Metadata:   nil,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	compressed := snappy.Encode(nil, data)
	return compressed, nil
}

func getSamples(compressed []byte) []prompb.TimeSeries {
	var samples []prompb.TimeSeries
	req := &prompb.WriteRequest{
		Timeseries: samples,
		Metadata:   nil,
	}

	d, _ := snappy.Decode(nil, compressed)
	proto.Unmarshal(d, req)

	return req.Timeseries
}
