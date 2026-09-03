package router

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

func extractMetricFromTimeSeries(s *prompb.TimeSeries) string {
	for i := 0; i < len(s.Labels); i++ {
		if s.Labels[i].Name == "__name__" {
			return s.Labels[i].Value
		}
	}
	return ""
}

// 返回的第二个参数，bool，表示是否需要把 ident 写入 target 表
func extractIdentFromTimeSeries(s *prompb.TimeSeries, ignoreIdent, ignoreHost bool, identMetrics []string) (string, bool) {
	if s == nil {
		return "", false
	}

	// 原实现为每条 TS 新建 map[string]int 定位 label 下标，pprof 显示仅 mapassign_faststr
	// 一项就占用约 13% CPU。改为一次线性扫描同时记录四个关键 label 的下标，零分配。
	identIdx, hostnameIdx, hostIdx, nameIdx := -1, -1, -1, -1
	for i := range s.Labels {
		switch s.Labels[i].Name {
		case "ident":
			identIdx = i
		case "agent_hostname":
			hostnameIdx = i
		case "host":
			hostIdx = i
		case "__name__":
			nameIdx = i
		}
	}

	var ident string

	// 如果标签中有ident，则直接使用
	if identIdx >= 0 {
		ident = s.Labels[identIdx].Value
	}

	if ident == "" && hostnameIdx >= 0 {
		// 没有 ident 标签，尝试使用 agent_hostname 作为 ident
		// agent_hostname for grafana-agent and categraf
		s.Labels[hostnameIdx].Name = "ident"
		ident = s.Labels[hostnameIdx].Value
	}

	if !ignoreHost && ident == "" && hostIdx >= 0 {
		// agent_hostname 没有，那就使用 host 作为 ident，用于 telegraf 的场景
		// 但是，有的时候 nginx 采集的指标中带有 host 标签表示域名，这个时候就不能用 host 作为 ident，此时需要在 url 中设置 ignore_host=true
		// telegraf, output plugin: http, format: prometheusremotewrite
		s.Labels[hostIdx].Name = "ident"
		ident = s.Labels[hostIdx].Value
	}

	if ident == "" {
		// 上报的监控数据中并没有 ident 信息
		return "", false
	}

	if len(identMetrics) > 0 {
		metricFound := false
		if nameIdx >= 0 {
			metricName := s.Labels[nameIdx].Value
			for _, identMetric := range identMetrics {
				if metricName == identMetric {
					metricFound = true
					break
				}
			}
		}
		if !metricFound {
			return ident, false
		}
	}

	return ident, !ignoreIdent
}

// duplicateLabelKeyLinearThreshold 控制线性 vs map 两种去重策略的切换阈值。
// n <= 阈值：O(n^2) 嵌套扫描，零分配，对 remote_write 典型 <20 label 的场景比 map 快得多；
// n  > 阈值：退回 map 去重，避免异常/恶意请求塞入大量 label 时被 O(n^2) 放大成 DoS。
// 阈值取 64：64*63/2 ≈ 2k 次指针比较，仍远低于构造一个 64-bucket map 的开销。
const duplicateLabelKeyLinearThreshold = 64

// duplicateLabelKey 判断是否有重复的 label 名称。
// 原实现始终新建 map[string]struct{}，pprof 显示占 ~9% CPU（每条 TS 都分配）。
func duplicateLabelKey(series *prompb.TimeSeries) bool {
	if series == nil {
		return false
	}
	labels := series.Labels
	n := len(labels)
	if n <= duplicateLabelKeyLinearThreshold {
		for i := 0; i < n; i++ {
			name := labels[i].Name
			for j := i + 1; j < n; j++ {
				if labels[j].Name == name {
					return true
				}
			}
		}
		return false
	}

	labelKeys := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		if _, has := labelKeys[labels[i].Name]; has {
			return true
		}
		labelKeys[labels[i].Name] = struct{}{}
	}
	return false
}

func (rt *Router) remoteWrite(c *gin.Context) {
	curLen := rt.Writers.AllQueueLen.Load().(int64)
	if curLen > rt.Pushgw.WriterOpt.AllQueueMaxSize {
		err := fmt.Errorf("write queue full, metric count over limit: %d", curLen)
		logger.Warning(err)
		pstat.CounterPushQueueOverLimitTotal.Inc()
		c.String(rt.Pushgw.WriterOpt.OverLimitStatusCode, err.Error())
		return
	}

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

	queueid := fmt.Sprint(atomic.AddUint64(&globalCounter, 1) % uint64(rt.Pushgw.WriterOpt.QueueNumber))

	var (
		ignoreIdent = ginx.QueryBool(c, "ignore_ident", false)
		ignoreHost  = ginx.QueryBool(c, "ignore_host", true) // 默认值改成 true，要不然答疑成本太高。发版的时候通知 telegraf 用户，让他们设置 ignore_host=false
		ids         = make(map[string]struct{})
	)

	for i := 0; i < count; i++ {
		if duplicateLabelKey(&req.Timeseries[i]) {
			continue
		}

		ident, insertTarget := extractIdentFromTimeSeries(&req.Timeseries[i], ignoreIdent, ignoreHost, rt.Pushgw.IdentMetrics)
		if len(ident) > 0 {
			// enrich host labels
			target, has := rt.TargetCache.Get(ident)
			if has {
				rt.AppendLabels(&req.Timeseries[i], target, rt.BusiGroupCache)
			}

			pstat.CounterSampleReceivedByIdent.WithLabelValues(ident).Inc()
		}

		if rt.Pushgw.GetHeartbeatFromMetric && insertTarget {
			// has ident tag or agent_hostname tag
			// register host in table target
			ids[ident] = struct{}{}
		}

		err = rt.ForwardToQueue(c.ClientIP(), queueid, &req.Timeseries[i])
		if err != nil {
			c.String(rt.Pushgw.WriterOpt.OverLimitStatusCode, err.Error())
			return
		}
	}

	pstat.CounterSampleTotal.WithLabelValues("prometheus").Add(float64(count))
	rt.IdentSet.MSet(ids)

	c.String(200, "")
}

// decodeBodyBufPool 缓存 HTTP body 读取缓冲，典型 snappy 压缩后 remote_write 批量在 ~64KB-256KB 之间。
// decodeSnappyBufPool 缓存 snappy 解压后的明文缓冲；过大的 buffer（>maxPooledBufCap）不回收以避免长期占用内存。
var (
	decodeBodyBufPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, 64*1024)
			return &b
		},
	}
	decodeSnappyBufPool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, 256*1024)
			return &b
		},
	}
)

const maxPooledBufCap = 4 * 1024 * 1024

// DecodeWriteRequest from an io.Reader into a prompb.WriteRequest, handling
// snappy decompression. 内部的 body 读取缓冲与 snappy 解码缓冲均从 sync.Pool 复用，
// 返回的 *WriteRequest 在本函数返回后仍可安全使用，因为 prompb.Unmarshal 会把
// label/sample 字段拷出到独立分配的 string。
func DecodeWriteRequest(r io.Reader) (*prompb.WriteRequest, error) {
	bodyBufP := decodeBodyBufPool.Get().(*[]byte)
	defer func() {
		if cap(*bodyBufP) <= maxPooledBufCap {
			decodeBodyBufPool.Put(bodyBufP)
		}
	}()

	buf := bytes.NewBuffer((*bodyBufP)[:0])
	if _, err := io.Copy(buf, r); err != nil {
		return nil, err
	}
	compressed := buf.Bytes()
	*bodyBufP = compressed

	dLen, err := snappy.DecodedLen(compressed)
	if err != nil {
		return nil, err
	}

	snappyBufP := decodeSnappyBufPool.Get().(*[]byte)
	defer func() {
		if cap(*snappyBufP) <= maxPooledBufCap {
			decodeSnappyBufPool.Put(snappyBufP)
		}
	}()
	snappyBuf := *snappyBufP
	if cap(snappyBuf) < dLen {
		snappyBuf = make([]byte, dLen)
	} else {
		snappyBuf = snappyBuf[:dLen]
	}
	*snappyBufP = snappyBuf

	reqBuf, err := snappy.Decode(snappyBuf, compressed)
	if err != nil {
		return nil, err
	}

	req := &prompb.WriteRequest{}
	if err := proto.Unmarshal(reqBuf, req); err != nil {
		return nil, err
	}

	return req, nil
}
