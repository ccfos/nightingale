package router

import (
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/server/common"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/idents"
	"github.com/didi/nightingale/v5/src/server/memsto"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
	"github.com/didi/nightingale/v5/src/server/writer"
)

var promMetricFilter map[string]bool = map[string]bool{
	"up":                                    true,
	"scrape_series_added":                   true,
	"scrape_samples_post_metric_relabeling": true,
	"scrape_samples_scraped":                true,
	"scrape_duration_seconds":               true,
}

type promqlForm struct {
	PromQL string `json:"promql"`
}

func queryPromql(c *gin.Context) {
	var f promqlForm
	ginx.BindJSON(c, &f)

	if config.ReaderClients.IsNil(config.C.ClusterName) {
		c.String(500, "reader client is nil")
		return
	}

	value, warnings, err := config.ReaderClients.GetCli(config.C.ClusterName).Query(c.Request.Context(), f.PromQL, time.Now())
	if err != nil {
		c.String(500, "promql:%s error:%v", f.PromQL, err)
		return
	}

	if len(warnings) > 0 {
		c.String(500, "promql:%s warnings:%v", f.PromQL, warnings)
		return
	}

	c.JSON(200, conv.ConvertVectors(value))
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

func remoteWrite(c *gin.Context) {
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
		now    = time.Now().Unix()
		ids    = make(map[string]interface{})
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
			if req.Timeseries[i].Labels[j].Name == "ident" {
				ident = req.Timeseries[i].Labels[j].Value
			} else if req.Timeseries[i].Labels[j].Name == "host" {
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

		// 当数据是通过prometheus抓取（也许直接remote write到夜莺）的时候，prometheus会自动产生部分系统指标
		// 例如最典型的有up指标，是prometheus为exporter生成的指标，即使exporter挂掉的时候也会送up=0的指标
		// 此类指标当剔除，否则会导致redis数据中时间戳被意外更新，导致由此类指标中携带的ident的相关target_up指标无法变为实际的0值
		// 更多详细信息：https://prometheus.io/docs/concepts/jobs_instances/#automatically-generated-labels-and-time-series
		if _, has := promMetricFilter[metric]; has {
			ident = ""
		}

		if len(ident) > 0 {
			// register host
			ids[ident] = now

			// fill tags
			target, has := memsto.TargetCache.Get(ident)
			if has {
				common.AppendLabels(req.Timeseries[i], target)
			}
		}

		LogSample(c.Request.RemoteAddr, req.Timeseries[i])

		if config.C.WriterOpt.ShardingKey == "ident" {
			if ident == "" {
				writer.Writers.PushSample("-", req.Timeseries[i])
			} else {
				writer.Writers.PushSample(ident, req.Timeseries[i])
			}
		} else {
			writer.Writers.PushSample(metric, req.Timeseries[i])
		}
	}

	cn := config.C.ClusterName
	if cn != "" {
		promstat.CounterSampleTotal.WithLabelValues(cn, "prometheus").Add(float64(count))
	}

	idents.Idents.MSet(ids)
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
