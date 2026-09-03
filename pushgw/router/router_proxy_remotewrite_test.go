package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// counterValue 读取一个 prometheus counter 的当前值，避免引入 testutil 包带来的额外依赖。
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// capture 线程安全地收集某个 writer 收到的所有 body，供并发转发断言。
type capture struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *capture) add(b []byte) {
	c.mu.Lock()
	c.bodies = append(c.bodies, append([]byte(nil), b...))
	c.mu.Unlock()
}

func (c *capture) snapshot() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.bodies...)
}

// startWriter 启动一个记录请求 body 的假 writer。sleep>0 模拟慢 writer，status>=400 模拟坏 writer。
func startWriter(t *testing.T, cap *capture, sleep time.Duration, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap.add(body)
		if sleep > 0 {
			time.Sleep(sleep)
		}
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newProxyRouter(concurrent bool, writers ...pconf.WriterOptions) *Router {
	rt := &Router{}
	rt.Pushgw.ProxyConcurrentForward = concurrent
	rt.Pushgw.Writers = writers
	// 走真实启动校验：补默认值并初始化每个 writer 的 HTTPTransport（否则 client.Do 会对 typed-nil transport 触发空指针）。
	rt.Pushgw.PreCheck()
	return rt
}

func invokeProxy(rt *Router, body []byte) int {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/proxy/v1/write", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/x-protobuf")
	c.Request.Header.Set("Content-Encoding", "snappy")
	rt.proxyRemoteWrite(c)
	return w.Code
}

// 验收 1：并行正确性——3 个 writer 各收到一份且 body 字节完全一致。
func TestProxyConcurrentForwardCorrectness(t *testing.T) {
	caps := []*capture{{}, {}, {}}
	writers := make([]pconf.WriterOptions, len(caps))
	for i := range caps {
		srv := startWriter(t, caps[i], 0, http.StatusOK)
		writers[i] = pconf.WriterOptions{Url: srv.URL, Timeout: 5000}
	}

	rt := newProxyRouter(true, writers...)
	body := bytes.Repeat([]byte("payload-bytes-"), 1000)

	if code := invokeProxy(rt, body); code != http.StatusOK {
		t.Fatalf("client code = %d, want 200", code)
	}

	for i, c := range caps {
		got := c.snapshot()
		if len(got) != 1 {
			t.Fatalf("writer %d received %d bodies, want 1", i, len(got))
		}
		if !bytes.Equal(got[0], body) {
			t.Fatalf("writer %d body mismatch: got %d bytes, want %d bytes", i, len(got[0]), len(body))
		}
	}
}

// 验收 2：并行加速——每个 writer sleep 200ms 时，并行整体耗时 ≈200ms，串行对照 ≈600ms。
func TestProxyConcurrentForwardSpeedup(t *testing.T) {
	const sleep = 200 * time.Millisecond
	const n = 3

	build := func() []pconf.WriterOptions {
		writers := make([]pconf.WriterOptions, n)
		for i := 0; i < n; i++ {
			srv := startWriter(t, &capture{}, sleep, http.StatusOK)
			writers[i] = pconf.WriterOptions{Url: srv.URL, Timeout: 5000}
		}
		return writers
	}

	body := []byte("hello")

	start := time.Now()
	invokeProxy(newProxyRouter(true, build()...), body)
	parallel := time.Since(start)

	start = time.Now()
	invokeProxy(newProxyRouter(false, build()...), body)
	serial := time.Since(start)

	// 并行应接近单个 writer 的延迟（留足调度余量），串行应接近 n 倍。
	if parallel >= sleep*2 {
		t.Fatalf("parallel took %v, want < %v", parallel, sleep*2)
	}
	if serial < sleep*time.Duration(n)-50*time.Millisecond {
		t.Fatalf("serial took %v, want >= ~%v", serial, sleep*time.Duration(n))
	}
	if parallel >= serial {
		t.Fatalf("parallel %v should be faster than serial %v", parallel, serial)
	}
}

// 验收 3：故障隔离——1 个 writer 返回 500 时，其余 writer 仍正常收到、客户端仍返回 200、
// 坏 writer 的转发错误计数 +1。
func TestProxyConcurrentForwardFaultIsolation(t *testing.T) {
	goodA, goodB, bad := &capture{}, &capture{}, &capture{}
	srvA := startWriter(t, goodA, 0, http.StatusOK)
	srvBad := startWriter(t, bad, 0, http.StatusInternalServerError)
	srvB := startWriter(t, goodB, 0, http.StatusOK)

	writers := []pconf.WriterOptions{
		{Url: srvA.URL, Timeout: 5000},
		{Url: srvBad.URL, Timeout: 5000},
		{Url: srvB.URL, Timeout: 5000},
	}
	rt := newProxyRouter(true, writers...)

	badErr := pstat.CounterProxyForwardErrorTotal.WithLabelValues(srvBad.URL, "status_4xx_5xx")
	before := counterValue(badErr)

	body := []byte("isolation-test-body")
	if code := invokeProxy(rt, body); code != http.StatusOK {
		t.Fatalf("client code = %d, want 200 despite a bad writer", code)
	}

	for name, c := range map[string]*capture{"goodA": goodA, "goodB": goodB} {
		got := c.snapshot()
		if len(got) != 1 || !bytes.Equal(got[0], body) {
			t.Fatalf("%s should still receive exactly one identical body, got %d", name, len(got))
		}
	}

	if after := counterValue(badErr); after-before != 1 {
		t.Fatalf("bad writer error counter delta = %v, want 1", after-before)
	}
}

// 验收 4：buffer pool 无竞争——并发多请求在 -race 下无 data race。
func TestProxyConcurrentForwardNoDataRace(t *testing.T) {
	caps := []*capture{{}, {}, {}}
	writers := make([]pconf.WriterOptions, len(caps))
	for i := range caps {
		srv := startWriter(t, caps[i], 5*time.Millisecond, http.StatusOK)
		writers[i] = pconf.WriterOptions{Url: srv.URL, Timeout: 5000}
	}
	rt := newProxyRouter(true, writers...)

	const requests = 50
	var wg sync.WaitGroup
	wg.Add(requests)
	for i := 0; i < requests; i++ {
		go func(i int) {
			defer wg.Done()
			body := bytes.Repeat([]byte{byte(i)}, 256)
			if code := invokeProxy(rt, body); code != http.StatusOK {
				t.Errorf("request %d code = %d, want 200", i, code)
			}
		}(i)
	}
	wg.Wait()

	for i, c := range caps {
		if got := len(c.snapshot()); got != requests {
			t.Fatalf("writer %d received %d bodies, want %d", i, got, requests)
		}
	}
}

// 验收 5：默认串行回归——不设开关时仍把同一份 body 分发给所有 writer、客户端返回 200。
func TestProxyDefaultSerialForward(t *testing.T) {
	caps := []*capture{{}, {}, {}}
	writers := make([]pconf.WriterOptions, len(caps))
	for i := range caps {
		srv := startWriter(t, caps[i], 0, http.StatusOK)
		writers[i] = pconf.WriterOptions{Url: srv.URL, Timeout: 5000}
	}

	rt := newProxyRouter(false, writers...) // 开关零值，默认串行
	body := []byte("serial-regression")

	if code := invokeProxy(rt, body); code != http.StatusOK {
		t.Fatalf("client code = %d, want 200", code)
	}

	for i, c := range caps {
		got := c.snapshot()
		if len(got) != 1 || !bytes.Equal(got[0], body) {
			t.Fatalf("writer %d should receive exactly one identical body, got %d", i, len(got))
		}
	}
}

// 单 writer 时即便开关打开也走串行分支，仍应正常转发。
func TestProxyConcurrentSingleWriter(t *testing.T) {
	cap := &capture{}
	srv := startWriter(t, cap, 0, http.StatusOK)
	rt := newProxyRouter(true, pconf.WriterOptions{Url: srv.URL, Timeout: 5000})

	body := []byte("single-writer")
	if code := invokeProxy(rt, body); code != http.StatusOK {
		t.Fatalf("client code = %d, want 200", code)
	}
	if got := cap.snapshot(); len(got) != 1 || !bytes.Equal(got[0], body) {
		t.Fatalf("single writer should receive exactly one identical body, got %d", len(got))
	}
}
