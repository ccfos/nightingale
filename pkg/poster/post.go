package poster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type DataResponse[T any] struct {
	Dat T      `json:"dat"`
	Err string `json:"err"`
}

// Shared HTTP client settings for edge → center calls.
//
// Prior to this, every Get/Post created a fresh &http.Client{Timeout: ...} with
// a nil Transport, which fell back to http.DefaultTransport. DefaultTransport is
// pooled, but its MaxIdleConnsPerHost default is only 2 — far too small for a
// pushgw/edge process that fans out many concurrent heartbeats, target-updates
// and cache syncs to a handful of center addresses. Requests beyond the idle
// cap could not be returned to the pool and degenerated into short-lived TCP
// connections, which is especially painful over HTTPS.
//
// We keep a package-level Transport with a larger per-host pool, and route
// per-request timeouts through context.WithTimeout instead of client.Timeout,
// so a single shared *http.Client can serve every caller.
const (
	posterMaxIdleConns          = 1024
	posterMaxIdleConnsPerHost   = 256
	posterMaxConnsPerHost       = 512
	posterIdleConnTimeout       = 90 * time.Second
	posterDialTimeout           = 5 * time.Second
	posterDialKeepAlive         = 30 * time.Second
	posterTLSHandshakeTimeout   = 10 * time.Second
	posterExpectContinueTimeout = 1 * time.Second

	// errBodyReadLimit caps how much of a non-2xx response body we surface in
	// the returned error message. The body is still drained beyond this limit
	// (see DrainAndReadErrBody) so the connection can be returned to the idle
	// pool; the limit only bounds how much we keep in memory / show to the
	// caller. errBodyDrainLimit is a hard cap on draining: past this size we
	// give up on connection reuse rather than blocking indefinitely on a
	// misbehaving server.
	errBodyReadLimit  = 64 * 1024
	errBodyDrainLimit = 1 << 20 // 1 MiB
)

var sharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   posterDialTimeout,
		KeepAlive: posterDialKeepAlive,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          posterMaxIdleConns,
	MaxIdleConnsPerHost:   posterMaxIdleConnsPerHost,
	MaxConnsPerHost:       posterMaxConnsPerHost,
	IdleConnTimeout:       posterIdleConnTimeout,
	TLSHandshakeTimeout:   posterTLSHandshakeTimeout,
	ExpectContinueTimeout: posterExpectContinueTimeout,
}

var sharedClient = &http.Client{
	Transport: sharedTransport,
}

// SharedClient returns the package-level keep-alive HTTP client used by the
// generic GetByUrl/PostByUrl helpers. Other packages that issue edge → center
// requests but cannot use the DataResponse[T] envelope (e.g. pushgw collect)
// can call this directly to share the same connection pool.
func SharedClient() *http.Client { return sharedClient }

// pickClient returns the shared keep-alive client for normal edge → center
// traffic, or a proxy-aware client when the target URL matches N9E_PROXY_URL.
// The proxy path is intentionally left on the legacy ProxyTransporter to keep
// behavior unchanged for webhook/notify use cases.
func pickClient(rawURL string) *http.Client {
	if UseProxy(rawURL) {
		return &http.Client{Transport: ProxyTransporter}
	}
	return sharedClient
}

// PathLabel extracts a bounded-cardinality path from a full URL for use as a
// Prometheus label. Query string and host are stripped. When parsing fails or
// the URL has no path component, "unknown" is returned and the raw URL is
// logged at debug level so operators can trace metric spikes back to the
// caller.
//
// Exported so other packages that issue edge → center HTTP traffic but cannot
// use GetByUrl/PostByUrl directly (e.g. callers that wrap http.Client or
// httputil.ReverseProxy themselves) can record into the same
// n9e_poster_request_* metric family with the same label semantics.
func PathLabel(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		logger.Debugf("poster: PathLabel falling back to \"unknown\" for url=%q err=%v", rawURL, err)
		return "unknown"
	}
	return u.Path
}

// ClassifyClientError maps a client.Do error into a coarse, bounded label so
// the "code" metric can distinguish "the request timed out" from "we never got
// a TCP connection" from "TLS handshake failed", without exploding label
// cardinality. The set of possible return values is fixed and small:
//
//	"timeout"  — request context deadline exceeded
//	"canceled" — request context canceled before a response
//	"neterror" — net.OpError (DNS, dial refused, reset, TLS, etc.)
//	"error"    — anything else
//
// Keep this in sync with the comment on the "code" label in metrics.go.
//
// Exported so out-of-package callers that wrap their own http.Client or
// RoundTripper can record the same coarse failure categories.
func ClassifyClientError(err error) string {
	if err == nil {
		return "error"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var ne *net.OpError
	if errors.As(err, &ne) {
		return "neterror"
	}
	return "error"
}

// DrainAndReadErrBody reads up to errBodyReadLimit bytes from body to surface
// in the returned error, then drains the rest (up to errBodyDrainLimit) so the
// underlying TCP connection can be returned to the idle pool. Without the
// drain step, http.Response.Body.Close on a partially-read body causes the
// Transport to discard the connection, defeating the connection-pool tuning
// this package was set up to enable. The drain itself is bounded by
// errBodyDrainLimit so a misbehaving server cannot block us indefinitely; if
// the body is larger than that, we accept losing the keep-alive for this
// request.
//
// The caller is still responsible for Close()-ing the body (typically via
// defer).
//
// Exported so out-of-package callers that issue their own HTTP requests can
// keep connections in the shared pool on non-2xx response paths.
func DrainAndReadErrBody(body io.Reader) string {
	bs, readErr := io.ReadAll(io.LimitReader(body, errBodyReadLimit))
	// Drain whatever remains so the connection can be reused. Cap the drain
	// so a hostile/broken server can't pin us here.
	_, _ = io.Copy(io.Discard, io.LimitReader(body, errBodyDrainLimit))
	if readErr != nil {
		return ""
	}
	return strings.TrimSpace(string(bs))
}

func GetByUrls[T any](ctx *ctx.Context, path string) (T, error) {
	addrs := ctx.CenterApi.Addrs
	if len(addrs) == 0 {
		var dat T
		return dat, fmt.Errorf("no center api addresses configured")
	}

	// 随机选择起始位置
	startIdx := rand.Intn(len(addrs))

	// 从随机位置开始遍历所有地址

	var dat T
	var err error
	for i := 0; i < len(addrs); i++ {
		idx := (startIdx + i) % len(addrs)
		url := fmt.Sprintf("%s%s", addrs[idx], path)

		dat, err = GetByUrl[T](url, ctx.CenterApi)
		if err != nil {
			logger.Warningf("failed to get data from center, url: %s, err: %v", url, err)
			continue
		}
		return dat, nil
	}

	return dat, fmt.Errorf("failed to get data from center, path= %s, addrs= %v err: %v", path, addrs, err)
}

func GetByUrl[T any](rawURL string, cfg conf.CenterApi) (T, error) {
	var dat T

	if cfg.Timeout < 1 {
		cfg.Timeout = 5000
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", rawURL, nil)
	if err != nil {
		return dat, fmt.Errorf("failed to create request: %w", err)
	}

	if len(cfg.BasicAuthUser) > 0 {
		req.SetBasicAuth(cfg.BasicAuthUser, cfg.BasicAuthPass)
	}

	client := pickClient(rawURL)

	// Metric observation is deferred so every return path is covered, including
	// future branches. codeLabel starts as "error" (no response received) and
	// is upgraded to a coarse failure category or the real HTTP status once we
	// know which one applies. The defer runs after resp.Body.Close() below,
	// which means it captures the full end-to-end duration including body drain.
	path := PathLabel(rawURL)
	start := time.Now()
	codeLabel := "error"
	defer func() { ObserveRequest(path, codeLabel, start) }()

	resp, err := client.Do(req)
	if err != nil {
		codeLabel = ClassifyClientError(err)
		return dat, fmt.Errorf("failed to fetch from url: %w", err)
	}
	defer resp.Body.Close()

	codeLabel = strconv.Itoa(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		errBody := DrainAndReadErrBody(resp.Body)
		return dat, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, errBody)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return dat, fmt.Errorf("failed to read response body: %w", err)
	}

	var dataResp DataResponse[T]
	err = json.Unmarshal(body, &dataResp)
	if err != nil {
		return dat, fmt.Errorf("failed to decode:%s response: %w", string(body), err)
	}

	if dataResp.Err != "" {
		return dat, fmt.Errorf("error from server: %s", dataResp.Err)
	}

	logger.Debugf("get data from %s, data: %+v", rawURL, dataResp.Dat)
	return dataResp.Dat, nil
}

func PostByUrls(ctx *ctx.Context, path string, v interface{}) error {
	addrs := ctx.CenterApi.Addrs
	if len(addrs) == 0 {
		return fmt.Errorf("submission of the POST request from the center has failed, "+
			"path= %s, v= %v, ctx.CenterApi.Addrs= %v", path, v, addrs)
	}

	// 随机选择起始位置
	startIdx := rand.Intn(len(addrs))

	// 从随机位置开始遍历所有地址
	for i := 0; i < len(addrs); i++ {
		idx := (startIdx + i) % len(addrs)
		url := fmt.Sprintf("%s%s", addrs[idx], path)

		_, err := PostByUrl[interface{}](url, ctx.CenterApi, v)
		if err != nil {
			logger.Warningf("failed to post data to center, url: %s, err: %v", url, err)
			continue
		}
		return nil
	}

	return fmt.Errorf("failed to post data to center, path= %s, addrs= %v", path, addrs)
}

func PostByUrlsWithResp[T any](ctx *ctx.Context, path string, v interface{}) (t T, err error) {
	addrs := ctx.CenterApi.Addrs
	if len(addrs) < 1 {
		err = fmt.Errorf("submission of the POST request from the center has failed, "+
			"path= %s, v= %v, ctx.CenterApi.Addrs= %v", path, v, addrs)
		return
	}

	// 随机选择起始位置
	startIdx := rand.Intn(len(addrs))

	// 从随机位置开始遍历所有地址
	for i := 0; i < len(addrs); i++ {
		idx := (startIdx + i) % len(addrs)
		url := fmt.Sprintf("%s%s", addrs[idx], path)

		t, err = PostByUrl[T](url, ctx.CenterApi, v)
		if err != nil {
			logger.Warningf("failed to post data to center, url: %s, err: %v", url, err)
			continue
		}
		return t, nil
	}

	return t, fmt.Errorf("failed to post data to center, path= %s, addrs= %v err: %v", path, addrs, err)
}

func PostByUrl[T any](rawURL string, cfg conf.CenterApi, v interface{}) (t T, err error) {
	var bs []byte
	bs, err = json.Marshal(v)
	if err != nil {
		return
	}

	if cfg.Timeout < 1 {
		cfg.Timeout = 5000
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", rawURL, bytes.NewReader(bs))
	if err != nil {
		return t, fmt.Errorf("failed to create request %q: %w", rawURL, err)
	}
	req.Header.Set("Content-Type", "application/json")

	if len(cfg.BasicAuthUser) > 0 {
		req.SetBasicAuth(cfg.BasicAuthUser, cfg.BasicAuthPass)
	}

	client := pickClient(rawURL)

	// See GetByUrl for why metric observation is deferred.
	path := PathLabel(rawURL)
	start := time.Now()
	codeLabel := "error"
	defer func() { ObserveRequest(path, codeLabel, start) }()

	resp, err := client.Do(req)
	if err != nil {
		codeLabel = ClassifyClientError(err)
		return t, fmt.Errorf("failed to fetch from url: %w", err)
	}
	defer resp.Body.Close()

	codeLabel = strconv.Itoa(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		errBody := DrainAndReadErrBody(resp.Body)
		return t, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, errBody)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return t, fmt.Errorf("failed to read response body: %w", err)
	}

	var dataResp DataResponse[T]
	err = json.Unmarshal(body, &dataResp)
	if err != nil {
		return t, fmt.Errorf("failed to decode response: %w", err)
	}

	if dataResp.Err != "" {
		return t, fmt.Errorf("error from server: %s", dataResp.Err)
	}

	logger.Debugf("get data from %s, data: %+v", rawURL, dataResp.Dat)
	return dataResp.Dat, nil

}

var ProxyTransporter = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
}

func UseProxy(url string) bool {
	// N9E_PROXY_URL=oapi.dingtalk.com,feishu.com
	patterns := os.Getenv("N9E_PROXY_URL")
	if patterns != "" {
		// 说明要让某些 URL 走代理
		for _, u := range strings.Split(patterns, ",") {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}

			if strings.Contains(url, u) {
				return true
			}
		}
	}
	return false
}

func PostJSON(url string, timeout time.Duration, v interface{}, retries ...int) (response []byte, code int, err error) {
	var bs []byte

	bs, err = json.Marshal(v)
	if err != nil {
		return
	}

	client := http.Client{
		Timeout: timeout,
	}

	if UseProxy(url) {
		client.Transport = ProxyTransporter
	}

	newRequest := func() (*http.Request, error) {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(bs))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	var resp *http.Response

	if len(retries) > 0 {
		for i := 0; i < retries[0]; i++ {
			var req *http.Request
			req, err = newRequest()
			if err != nil {
				return
			}

			resp, err = client.Do(req)
			if err == nil {
				break
			}

			tryagain := ""
			if i+1 < retries[0] {
				tryagain = " try again"
			}

			logger.Warningf("failed to curl %s error: %s"+tryagain, url, err)

			if i+1 < retries[0] {
				time.Sleep(time.Millisecond * 200)
			}
		}
	} else {
		var req *http.Request
		req, err = newRequest()
		if err != nil {
			return
		}
		resp, err = client.Do(req)
	}

	if err != nil {
		return
	}

	code = resp.StatusCode

	if resp.Body != nil {
		defer resp.Body.Close()
		response, err = io.ReadAll(resp.Body)
	}

	return
}
