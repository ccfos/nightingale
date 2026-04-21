package router

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// 客户端把数据推给 pushgw，pushgw 再转发给 prometheus。
// 这个方法中，pushgw 不做任何处理，不解析 http request body，直接转发给配置文件中指定的多个 writers。
// 相比 /prometheus/v1/write 方法，这个方法不需要在内存里搞很多队列，性能更好。
//
// 背压策略：用 in-flight 并发计数做闸门，超过 ProxyInflightMax 直接返回 429。
// remote_write 协议原生支持客户端侧 WAL + 退避重试，把缓冲责任交回客户端是最干净的做法。

// proxyBodyBufPool 复用 HTTP body 读取 buffer，避免高 QPS 下每请求都 make 大 slice。
// 典型 snappy 压缩后 remote_write 批量在 64KB-256KB 之间。
var proxyBodyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 128*1024)
		return &b
	},
}

// proxyBodyBufMaxCap 过大的 buffer 不回收，避免长期占用内存。
const proxyBodyBufMaxCap = 4 * 1024 * 1024

// 全局 in-flight 计数。所有 pushgw 实例共享（进程内），用 atomic 操作。
var proxyInflight atomic.Int64

// proxyDrainOnRejectBytes 429 路径下的 body drain 上限。
// 小量 drain 可以在常见请求大小下保住 keep-alive；超过则放弃 drain 让 server 关连接，
// 避免被拒请求反而消耗大量 IO 加重过载。
const proxyDrainOnRejectBytes = 64 * 1024

func (rt *Router) proxyRemoteWrite(c *gin.Context) {
	pstat.CounterProxyRemoteWriteTotal.Inc()

	// 背压：CAS 抢占一个 in-flight slot。
	// 用 CAS 而不是 Add-then-check，被拒请求不会短暂把计数推到 max+N，gauge 不会出现毛刺。
	max := int64(rt.Pushgw.ProxyInflightMax)
	for {
		cur := proxyInflight.Load()
		if max > 0 && cur >= max {
			pstat.CounterProxyRemoteWriteOverLimitTotal.Inc()
			// 小量 drain 保住 keep-alive；大 body 直接放弃让 server 关连接
			io.Copy(io.Discard, io.LimitReader(c.Request.Body, proxyDrainOnRejectBytes))
			c.String(http.StatusTooManyRequests, "proxy remote write inflight over limit: %d", cur)
			return
		}
		if proxyInflight.CompareAndSwap(cur, cur+1) {
			pstat.GaugeProxyRemoteWriteInflight.Set(float64(cur + 1))
			break
		}
	}
	defer func() {
		pstat.GaugeProxyRemoteWriteInflight.Set(float64(proxyInflight.Add(-1)))
	}()

	// 从 pool 取 buffer 读取 body
	bufP := proxyBodyBufPool.Get().(*[]byte)
	defer func() {
		if cap(*bufP) <= proxyBodyBufMaxCap {
			*bufP = (*bufP)[:0]
			proxyBodyBufPool.Put(bufP)
		}
	}()

	// 限制单请求 body 大小，防止异常/恶意客户端把 pushgw 打爆。
	// 多读 1 字节用来区分"刚好等于上限"和"超过上限"。
	maxBody := rt.Pushgw.ProxyMaxBodyBytes
	limited := io.LimitReader(c.Request.Body, maxBody+1)

	buf := bytes.NewBuffer((*bufP)[:0])
	n, err := io.Copy(buf, limited)
	if err != nil {
		// body 可能已损坏，只做小量 drain 保 keep-alive，不做无限 drain 避免被客户端利用
		io.Copy(io.Discard, io.LimitReader(c.Request.Body, proxyDrainOnRejectBytes))
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if n > maxBody {
		pstat.CounterProxyRemoteWriteBodyTooLargeTotal.Inc()
		// 超限时不再 drain 全部剩余 body：若 Content-Length 很大，drain 就是 DoS 放大器，
		// 正好绕开 ProxyMaxBodyBytes 保护。小量 drain 尽力保 keep-alive；超过就让 server 关连接。
		io.Copy(io.Discard, io.LimitReader(c.Request.Body, proxyDrainOnRejectBytes))
		c.String(http.StatusRequestEntityTooLarge, "proxy remote write body too large: > %d bytes", maxBody)
		return
	}
	bs := buf.Bytes()
	*bufP = bs

	// 透传 header
	contentType := c.GetHeader("Content-Type")
	if contentType == "" {
		contentType = "application/x-protobuf"
	}
	contentEncoding := c.GetHeader("Content-Encoding")
	if contentEncoding == "" {
		contentEncoding = "snappy"
	}
	userAgent := c.GetHeader("User-Agent")
	if userAgent == "" {
		userAgent = "n9e"
	} else {
		userAgent += "-n9e"
	}
	rwVersion := c.GetHeader("X-Prometheus-Remote-Write-Version")
	if rwVersion == "" {
		rwVersion = "0.1.0"
	}

	rawQuery := c.Request.URL.RawQuery

	// 串行转发给所有 writer。
	// 并行可以把耗时从 sum(latency) 降到 max(latency)，但会放大"任一慢 writer 拖累所有请求"的木桶效应；
	// 当前在 (a) 背压保护下，串行足以扛住慢后端——慢请求只会占用 in-flight slot，到阈值后新请求被 429 拒掉。
	for index := range rt.Pushgw.Writers {
		writer := rt.Pushgw.Writers[index]
		rt.forwardToWriter(bs, writer, rawQuery, contentType, contentEncoding, userAgent, rwVersion)
	}
}

// forwardToWriter 转发一次到单个 writer。抽成独立函数以便 defer res.Body.Close() 随函数返回立即执行，
// 避免原实现在 for 循环里 defer、所有 response body 堆积到 handler 返回才关闭导致的连接泄漏。
func (rt *Router) forwardToWriter(bs []byte, w pconf.WriterOptions, rawQuery, contentType, contentEncoding, userAgent, rwVersion string) {
	targetUrl := w.Url
	if rawQuery != "" {
		if strings.Contains(w.Url, "?") {
			targetUrl += "&" + rawQuery
		} else {
			targetUrl += "?" + rawQuery
		}
	}

	pstat.CounterProxyForwardTotal.WithLabelValues(w.Url).Inc()
	start := time.Now()
	defer func() {
		pstat.ProxyForwardDuration.WithLabelValues(w.Url).Observe(time.Since(start).Seconds())
	}()

	req, err := http.NewRequest("POST", targetUrl, bytes.NewReader(bs))
	if err != nil {
		pstat.CounterProxyForwardErrorTotal.WithLabelValues(w.Url, "build_request").Inc()
		logger.Warningf("[forward-timeseries] build request failed. url=%s error=%v", targetUrl, err)
		return
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Encoding", contentEncoding)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Prometheus-Remote-Write-Version", rwVersion)

	if w.BasicAuthUser != "" {
		req.SetBasicAuth(w.BasicAuthUser, w.BasicAuthPass)
	}

	headerCount := len(w.Headers)
	if headerCount > 0 && headerCount%2 == 0 {
		for i := 0; i < len(w.Headers); i += 2 {
			req.Header.Add(w.Headers[i], w.Headers[i+1])
			if w.Headers[i] == "Host" {
				req.Host = w.Headers[i+1]
			}
		}
	}

	client := http.Client{
		Timeout:   time.Duration(w.Timeout) * time.Millisecond,
		Transport: w.HTTPTransport,
	}

	res, err := client.Do(req)
	if err != nil {
		pstat.CounterProxyForwardErrorTotal.WithLabelValues(w.Url, "do_request").Inc()
		logger.Warningf("[forward-timeseries] failed to do request. url=%s error=%v", targetUrl, err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		pstat.CounterProxyForwardErrorTotal.WithLabelValues(w.Url, "status_4xx_5xx").Inc()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			logger.Warningf("[forward-timeseries] failed to read response body. url=%s error=%v", targetUrl, err)
			return
		}
		logger.Warningf("[forward-timeseries] response status code ge 400. url=%s status_code=%d response=%s", targetUrl, res.StatusCode, string(body))
		return
	}

	// 把 body 读干净再关闭，确保 keep-alive 连接能归还到连接池
	io.Copy(io.Discard, res.Body)
}
