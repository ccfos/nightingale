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

		// 当数据是通过prometheus抓取（也许直接remote write到夜莺）的时候，prometheus会自动产生部分系统指标
		// 例如最典型的有up指标，是prometheus为exporter生成的指标，即使exporter挂掉的时候也会送up=0的指标
		// 此类指标当剔除，否则会导致redis数据中时间戳被意外更新，导致由此类指标中携带的ident的相关target_up指标无法变为实际的0值
		// 更多详细信息：https://prometheus.io/docs/concepts/jobs_instances/#automatically-generated-labels-and-time-series
		if _, has := promMetricFilter[metric]; has {
			ident = ""
		}

		if len(ident) > 0 {
			// register host
			ids[ident] = struct{}{}

			// fill tags
			target, has := rt.TargetCache.Get(ident)
			if has {
				rt.AppendLabels(req.Timeseries[i], target, rt.BusiGroupCache)
			}
		}

		rt.EnrichLabels(req.Timeseries[i])
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
