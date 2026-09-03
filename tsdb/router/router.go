// Package router exposes prometheus compatible query endpoints backed by the
// embedded tsdb, mounted at /prometheus/api/v1/* so the auto-registered
// datasource url http(s)://<host>:<port>/prometheus works with both the
// frontend datasource proxy and the alert engine prometheus client. By
// default only requests from the n9e host itself are accepted; configuring
// BasicAuth or DatasourceUrl allows remote access (see Router.localOnly).
package router

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/ccfos/nightingale/v6/tsdb"

	"github.com/gin-gonic/gin"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// same defaults as prometheus web/api/v1: effectively unbounded time range
var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 0).UTC()
)

const (
	// maxWriteBodyBytes caps a single remote write request, both the
	// compressed body and the snappy-decoded payload. The snappy header can
	// declare up to 4GiB, so without this a few dozen bytes are enough to make
	// the process allocate itself to death. Same value as the default of
	// Pushgw.ProxyMaxBodyBytes, which is orders of magnitude above what the
	// pushgw writer actually sends (QueuePopSize series per batch).
	maxWriteBodyBytes = 32 << 20

	// maxWriteConcurrency bounds how many remote write requests are decoded and
	// appended at the same time, so peak memory stays bounded regardless of how
	// many clients push concurrently. Extra requests queue instead of being
	// rejected: the pushgw writer treats 4xx as success and would drop the
	// samples (see pushgw/writer.Post).
	maxWriteConcurrency = 64

	// defaultConcurrency is a fallback for a Cfg that never went through
	// tconf.EmbeddedTSDB.PreCheck, so the semaphores below never end up
	// unbuffered (which would block every request until its client gives up).
	defaultConcurrency = 20
)

type Router struct {
	inst *tsdb.Instance
	// localOnly: without BasicAuth these endpoints expose full metrics read
	// and unauthenticated sample injection to anyone who can reach the http
	// port, so by default only requests from this machine are accepted. The
	// local writer/datasource/frontend-proxy/alert-engine chain all originate
	// locally; configuring BasicAuthUser/Pass (or an explicit DatasourceUrl)
	// states the intent of remote access and lifts the restriction.
	localOnly bool
	// bindIP is the non-wildcard address the http server is bound to, if any.
	// A local process connecting to it gets that address as its source (the
	// traffic still goes through loopback), so requireLocal must accept it
	// alongside 127.0.0.1/::1.
	bindIP net.IP
	// scanSem bounds the concurrency of the endpoints that scan the index
	// directly (series/labels/label values, and delete when enabled),
	// bypassing the promql engine's ActiveQueryTracker.
	scanSem chan struct{}
	// writeSem bounds the concurrency of the remote write endpoint.
	writeSem chan struct{}
}

// New builds the router. httpHost is the HTTP.Host the server binds to, used
// by the local-only check; pass "" when unknown (wildcard bind).
func New(inst *tsdb.Instance, httpHost string) *Router {
	scanConcurrency := inst.Cfg.QueryMaxConcurrency
	if scanConcurrency <= 0 {
		scanConcurrency = defaultConcurrency
	}

	var bindIP net.IP
	if ip := net.ParseIP(httpHost); ip != nil && !ip.IsUnspecified() {
		bindIP = ip
	}

	return &Router{
		inst:      inst,
		localOnly: inst.Cfg.BasicAuthUser == "" && inst.Cfg.DatasourceUrl == "",
		bindIP:    bindIP,
		scanSem:   make(chan struct{}, scanConcurrency),
		writeSem:  make(chan struct{}, maxWriteConcurrency),
	}
}

func (rt *Router) Config(r *gin.Engine) {
	g := r.Group("/prometheus")

	cfg := rt.inst.Cfg
	if rt.localOnly {
		g.Use(rt.requireLocal)
	}
	if cfg.BasicAuthUser != "" {
		g.Use(gin.BasicAuth(gin.Accounts{cfg.BasicAuthUser: cfg.BasicAuthPass}))
	}

	// prometheus remote write receiver. pushgw forwards samples here through
	// an auto-injected [[Pushgw.Writers]] entry (see conf.InitCenterConfig);
	// external producers (categraf/prometheus agent) can also write directly
	// once BasicAuth is configured (the default is local-only, see above)
	g.POST("/api/v1/write", rt.limitWrite, rt.remoteWrite)

	g.GET("/api/v1/query", rt.query)
	g.POST("/api/v1/query", rt.query)
	g.GET("/api/v1/query_range", rt.queryRange)
	g.POST("/api/v1/query_range", rt.queryRange)
	g.GET("/api/v1/series", rt.limitScan, rt.series)
	g.POST("/api/v1/series", rt.limitScan, rt.series)
	g.GET("/api/v1/labels", rt.limitScan, rt.labelNames)
	g.POST("/api/v1/labels", rt.limitScan, rt.labelNames)
	g.GET("/api/v1/label/:name/values", rt.limitScan, rt.labelValues)
	g.GET("/api/v1/status/buildinfo", rt.buildInfo)

	// destructive admin endpoints, off by default like prometheus
	// --web.enable-admin-api; when off they are not registered at all
	if cfg.EnableAdminAPI {
		g.POST("/api/v1/admin/tsdb/delete_series", rt.limitScan, rt.deleteSeries)
		g.PUT("/api/v1/admin/tsdb/delete_series", rt.limitScan, rt.deleteSeries)
		g.POST("/api/v1/admin/tsdb/clean_tombstones", rt.cleanTombstones)
		g.PUT("/api/v1/admin/tsdb/clean_tombstones", rt.cleanTombstones)
	}
}

// requireLocal rejects requests that don't originate from this machine, the
// zero-config default (see Router.localOnly). Judged by the connection's
// RemoteAddr — never by forwardable headers.
func (rt *Router) requireLocal(c *gin.Context) {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || (rt.bindIP != nil && ip.Equal(rt.bindIP))) {
			c.Next()
			return
		}
	}

	respondError(c, http.StatusForbidden, "forbidden",
		"embedded tsdb endpoints only accept requests from the n9e host by default; "+
			"set EmbeddedTSDB.BasicAuthUser/BasicAuthPass (or DatasourceUrl) to allow remote access")
	c.Abort()
}

// limitScan queues the request until a concurrency slot frees up, giving up
// when the client goes away.
func (rt *Router) limitScan(c *gin.Context) {
	rt.limit(c, rt.scanSem, "query")
}

// limitWrite is limitScan for the remote write endpoint.
func (rt *Router) limitWrite(c *gin.Context) {
	rt.limit(c, rt.writeSem, "write")
}

func (rt *Router) limit(c *gin.Context, sem chan struct{}, kind string) {
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
		c.Next()
	case <-c.Request.Context().Done():
		respondError(c, http.StatusServiceUnavailable, "unavailable", "canceled while waiting for a "+kind+" slot")
		c.Abort()
	}
}

func (rt *Router) remoteWrite(c *gin.Context) {
	compressed, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxWriteBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			respondError(c, http.StatusRequestEntityTooLarge, "bad_data",
				fmt.Sprintf("request body exceeds the %d bytes limit", maxWriteBodyBytes))
			return
		}
		respondError(c, http.StatusBadRequest, "bad_data", "failed to read body: "+err.Error())
		return
	}

	// check the length declared in the snappy header before decoding, otherwise
	// a tiny crafted body makes snappy.Decode allocate up to 4GiB in one shot
	decodedLen, err := snappy.DecodedLen(compressed)
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "failed to read snappy header: "+err.Error())
		return
	}

	if decodedLen > maxWriteBodyBytes {
		respondError(c, http.StatusRequestEntityTooLarge, "bad_data",
			fmt.Sprintf("decoded body size %d exceeds the %d bytes limit", decodedLen, maxWriteBodyBytes))
		return
	}

	data, err := snappy.Decode(nil, compressed)
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "failed to snappy decode body: "+err.Error())
		return
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(data, &req); err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "failed to unmarshal write request: "+err.Error())
		return
	}

	if err := rt.inst.AppendTimeSeries(req.Timeseries); err != nil {
		respondError(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

func respondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

func respondError(c *gin.Context, code int, errType, errMsg string) {
	c.JSON(code, gin.H{"status": "error", "errorType": errType, "error": errMsg})
}

func (rt *Router) query(c *gin.Context) {
	qs := c.Request.FormValue("query")
	if qs == "" {
		respondError(c, http.StatusBadRequest, "bad_data", "query parameter is required")
		return
	}

	ts := time.Now()
	if v := c.Request.FormValue("time"); v != "" {
		var err error
		if ts, err = parseTime(v); err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter time: "+err.Error())
			return
		}
	}

	qry, err := rt.inst.Engine.NewInstantQuery(c.Request.Context(), rt.inst.DB, nil, qs, ts)
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	defer qry.Close()

	res := qry.Exec(c.Request.Context())
	if res.Err != nil {
		respondError(c, http.StatusUnprocessableEntity, "execution", res.Err.Error())
		return
	}

	respondOK(c, gin.H{"resultType": res.Value.Type(), "result": res.Value})
}

func (rt *Router) queryRange(c *gin.Context) {
	qs := c.Request.FormValue("query")
	if qs == "" {
		respondError(c, http.StatusBadRequest, "bad_data", "query parameter is required")
		return
	}

	start, err := parseTime(c.Request.FormValue("start"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter start: "+err.Error())
		return
	}

	end, err := parseTime(c.Request.FormValue("end"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter end: "+err.Error())
		return
	}

	if end.Before(start) {
		respondError(c, http.StatusBadRequest, "bad_data", "end timestamp must not be before start time")
		return
	}

	step, err := parseDuration(c.Request.FormValue("step"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter step: "+err.Error())
		return
	}

	if step <= 0 {
		respondError(c, http.StatusBadRequest, "bad_data", "step must be greater than zero")
		return
	}

	if end.Sub(start)/step > 11000 {
		respondError(c, http.StatusBadRequest, "bad_data", "exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution (?step=XX)")
		return
	}

	qry, err := rt.inst.Engine.NewRangeQuery(c.Request.Context(), rt.inst.DB, nil, qs, start, end, step)
	if err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", err.Error())
		return
	}
	defer qry.Close()

	res := qry.Exec(c.Request.Context())
	if res.Err != nil {
		respondError(c, http.StatusUnprocessableEntity, "execution", res.Err.Error())
		return
	}

	respondOK(c, gin.H{"resultType": res.Value.Type(), "result": res.Value})
}

func (rt *Router) series(c *gin.Context) {
	selectors, start, end, ok := rt.matchParams(c)
	if !ok {
		return
	}

	if len(selectors) == 0 {
		respondError(c, http.StatusBadRequest, "bad_data", "no match[] parameter provided")
		return
	}

	q, err := rt.inst.DB.Querier(timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer q.Close()

	// Func "series" tells the block querier this is a metadata lookup, so it
	// resolves labels from the index without reading any chunk (same as
	// prometheus web/api/v1)
	hints := &storage.SelectHints{
		Start: timestamp.FromTime(start),
		End:   timestamp.FromTime(end),
		Func:  "series",
	}

	seen := make(map[string]labels.Labels)
	for _, sel := range selectors {
		matchers, err := parser.ParseMetricSelector(sel)
		if err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter match[]: "+err.Error())
			return
		}

		set := q.Select(c.Request.Context(), false, hints, matchers...)
		for set.Next() {
			lset := set.At().Labels()
			seen[lset.String()] = lset
		}
		if err := set.Err(); err != nil {
			respondError(c, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]labels.Labels, 0, len(seen))
	for _, k := range keys {
		result = append(result, seen[k])
	}

	respondOK(c, result)
}

func (rt *Router) labelNames(c *gin.Context) {
	selectors, start, end, ok := rt.matchParams(c)
	if !ok {
		return
	}

	q, err := rt.inst.DB.Querier(timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer q.Close()

	if len(selectors) == 0 {
		names, _, err := q.LabelNames(c.Request.Context())
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if names == nil {
			names = []string{}
		}
		respondOK(c, names)
		return
	}

	set := make(map[string]struct{})
	for _, sel := range selectors {
		matchers, err := parser.ParseMetricSelector(sel)
		if err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter match[]: "+err.Error())
			return
		}

		names, _, err := q.LabelNames(c.Request.Context(), matchers...)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		for _, name := range names {
			set[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	respondOK(c, names)
}

func (rt *Router) labelValues(c *gin.Context) {
	name := c.Param("name")
	if !model.LabelName(name).IsValid() {
		respondError(c, http.StatusBadRequest, "bad_data", "invalid label name: "+name)
		return
	}

	selectors, start, end, ok := rt.matchParams(c)
	if !ok {
		return
	}

	q, err := rt.inst.DB.Querier(timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer q.Close()

	if len(selectors) == 0 {
		values, _, err := q.LabelValues(c.Request.Context(), name)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if values == nil {
			values = []string{}
		}
		respondOK(c, values)
		return
	}

	set := make(map[string]struct{})
	for _, sel := range selectors {
		matchers, err := parser.ParseMetricSelector(sel)
		if err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter match[]: "+err.Error())
			return
		}

		values, _, err := q.LabelValues(c.Request.Context(), name, matchers...)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		for _, v := range values {
			set[v] = struct{}{}
		}
	}

	values := make([]string, 0, len(set))
	for v := range set {
		values = append(values, v)
	}
	sort.Strings(values)
	respondOK(c, values)
}

func (rt *Router) buildInfo(c *gin.Context) {
	respondOK(c, gin.H{
		"version":   "2.49.1",
		"revision":  "n9e-embedded-tsdb",
		"branch":    "HEAD",
		"buildUser": "",
		"buildDate": "",
		"goVersion": runtime.Version(),
	})
}

func (rt *Router) deleteSeries(c *gin.Context) {
	selectors, start, end, ok := rt.matchParams(c)
	if !ok {
		return
	}

	if len(selectors) == 0 {
		respondError(c, http.StatusBadRequest, "bad_data", "no match[] parameter provided")
		return
	}

	for _, sel := range selectors {
		matchers, err := parser.ParseMetricSelector(sel)
		if err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter match[]: "+err.Error())
			return
		}

		if err := rt.inst.DB.Delete(c.Request.Context(), timestamp.FromTime(start), timestamp.FromTime(end), matchers...); err != nil {
			respondError(c, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}

	c.Status(http.StatusNoContent)
}

func (rt *Router) cleanTombstones(c *gin.Context) {
	if err := rt.inst.DB.CleanTombstones(); err != nil {
		respondError(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// matchParams parses match[]/start/end shared by series, labels and admin
// endpoints. It writes the error response itself and returns ok=false on
// invalid input.
func (rt *Router) matchParams(c *gin.Context) (selectors []string, start, end time.Time, ok bool) {
	if err := c.Request.ParseForm(); err != nil {
		respondError(c, http.StatusBadRequest, "bad_data", "failed to parse form: "+err.Error())
		return nil, start, end, false
	}

	selectors = c.Request.Form["match[]"]

	start = minTime
	if v := c.Request.FormValue("start"); v != "" {
		var err error
		if start, err = parseTime(v); err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter start: "+err.Error())
			return nil, start, end, false
		}
	}

	end = maxTime
	if v := c.Request.FormValue("end"); v != "" {
		var err error
		if end, err = parseTime(v); err != nil {
			respondError(c, http.StatusBadRequest, "bad_data", "invalid parameter end: "+err.Error())
			return nil, start, end, false
		}
	}

	return selectors, start, end, true
}

// parseTime accepts unix timestamps (with optional fraction) and RFC3339,
// same as the prometheus HTTP API.
func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		sec, frac := math.Modf(t)
		return time.Unix(int64(sec), int64(frac*float64(time.Second))).UTC(), nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// parseDuration accepts seconds (float) and prometheus duration strings like
// 30s, 1m, 1h.
func parseDuration(s string) (time.Duration, error) {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(v * float64(time.Second)), nil
	}
	d, err := model.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(d), nil
}
