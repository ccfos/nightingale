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

var promMetricFilter map[string]bool = map[string]bool{
	"up":                                    true,
	"scrape_series_added":                   true,
	"scrape_samples_post_metric_relabeling": true,
	"scrape_samples_scraped":                true,
	"scrape_duration_seconds":               true,
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
		ids    = make(map[string]int64)
		ident  string
		metric string
	)

	for i := 0; i < count; i++ {
		if duplicateLabelKey(req.Timeseries[i]) {
			continue
		}

		ident = ""

		// find ident label
		for j := 0; j < len(req.Timeseries[i].Labels); j++ {
			if req.Timeseries[i].Labels[j].Name == "host" {
				req.Timeseries[i].Labels[j].Name = "ident"
			}

			if req.Timeseries[i].Labels[j].Name == "ident" {
				ident = req.Timeseries[i].Labels[j].Value
			}

			if req.Timeseries[i].Labels[j].Name == "__name__" {
				metric = req.Timeseries[i].Labels[j].Value
			}
		}

		if ident == "" {
			// not found, try agent_hostname
			for j := 0; j < len(req.Timeseries[i].Labels); j++ {
				// agent_hostname for grafana-agent
				if req.Timeseries[i].Labels[j].Name == "agent_hostname" {
					req.Timeseries[i].Labels[j].Name = "ident"
					ident = req.Timeseries[i].Labels[j].Value
				}
			}
		}

		if len(ident) > 0 {
			// register host
			// https://prometheus.io/docs/concepts/jobs_instances/#automatically-generated-labels-and-time-series
			if _, has := promMetricFilter[metric]; !has {
				ids[ident] = getTs(req.Timeseries[i])
			}

			// fill tags
			target, has := rt.TargetCache.Get(ident)
			if has {
				rt.AppendLabels(req.Timeseries[i], target, rt.BusiGroupCache)
			}
		}

		rt.debugSample(c.Request.RemoteAddr, req.Timeseries[i])

		if rt.Pushgw.WriterOpt.ShardingKey == "ident" {
			if ident == "" {
				rt.Writers.PushSample("-", req.Timeseries[i])
			} else {
				rt.Writers.PushSample(ident, req.Timeseries[i])
			}
		} else {
			rt.Writers.PushSample(metric, req.Timeseries[i])
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
