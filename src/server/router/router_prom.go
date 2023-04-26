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

		ident = extractIdentFromTimeSeries(req.Timeseries[i])

		for j := 0; j < len(req.Timeseries[i].Labels); j++ {
			if req.Timeseries[i].Labels[j].Name == "__name__" {
				metric = req.Timeseries[i].Labels[j].Value
			}
		}

		// telegraf 上报数据的场景，只有在 metric 为 system_load1 时，说明指标来自机器，将 host 改为 ident，其他情况都忽略
		if metric != "system_load1" {
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
