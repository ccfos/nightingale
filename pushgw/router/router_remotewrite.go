package router

import (
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func extractMetricFromTimeSeries(s *prompb.TimeSeries) string {
	for i := 0; i < len(s.Labels); i++ {
		if s.Labels[i].Name == "__name__" {
			return s.Labels[i].Value
		}
	}
	return ""
}

func extractIdentFromTimeSeries(s *prompb.TimeSeries) string {
	for i := 0; i < len(s.Labels); i++ {
		if s.Labels[i].Name == "ident" {
			return s.Labels[i].Value
		}
	}

	// agent_hostname for grafana-agent and categraf
	for i := 0; i < len(s.Labels); i++ {
		if s.Labels[i].Name == "agent_hostname" {
			s.Labels[i].Name = "ident"
			return s.Labels[i].Value
		}
	}

	// telegraf, output plugin: http, format: prometheusremotewrite
	for i := 0; i < len(s.Labels); i++ {
		if s.Labels[i].Name == "host" {
			s.Labels[i].Name = "ident"
			return s.Labels[i].Value
		}
	}

	return ""
}

func duplicateLabelKey(series *prompb.TimeSeries) bool {
	if series == nil {
		return false
	}

	labelKeys := make(map[string]struct{})

	for j := 0; j < len(series.Labels); j++ {
		if _, has := labelKeys[series.Labels[j].Name]; has {
			return true
		} else {
			labelKeys[series.Labels[j].Name] = struct{}{}
		}
	}

	return false
}

func (rt *Router) remoteWrite(c *gin.Context) {
	req, err := DecodeWriteRequest(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	count := len(req.Timeseries)

	if count == 0 {
		c.String(200, "")
		return
	}

	var (
		ident  string
		metric string
		ids    = make(map[string]struct{})
	)

	for i := 0; i < count; i++ {
		if duplicateLabelKey(req.Timeseries[i]) {
			continue
		}

		ident = extractIdentFromTimeSeries(req.Timeseries[i])
		if len(ident) > 0 {
			// has ident tag or agent_hostname tag
			// register host in table target
			ids[ident] = struct{}{}

			// enrich host labels
			target, has := rt.TargetCache.Get(ident)
			if has {
				rt.AppendLabels(req.Timeseries[i], target, rt.BusiGroupCache)
			}
		}

		rt.EnrichLabels(req.Timeseries[i])
		rt.debugSample(c.Request.RemoteAddr, req.Timeseries[i])

		if len(ident) > 0 {
			// use ident as hash key, cause "out of bounds" problem
			rt.Writers.PushSample(ident, req.Timeseries[i])
		} else {
			// no ident tag, use metric name as hash key
			// sharding again cause there are too many series with the same metric name
			metric = extractMetricFromTimeSeries(req.Timeseries[i])
			var hashkey string
			if len(metric) >= 2 {
				hashkey = metric[0:2]
			} else {
				hashkey = metric[0:1]
			}

			rt.Writers.PushSample(hashkey, req.Timeseries[i])
		}
	}

	CounterSampleTotal.WithLabelValues("prometheus").Add(float64(count))
	rt.IdentSet.MSet(ids)
}

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression.
func DecodeWriteRequest(r io.Reader) (*prompb.WriteRequest, error) {
	compressed, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		return nil, err
	}

	return &req, nil
}
