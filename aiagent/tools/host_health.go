package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

// Status buckets shared by realtime / neighbor tools — keep the same thresholds
// the platform UI uses (router_target.go:137-141): <60s active, <180s lagging,
// otherwise stale. "stale_no_heartbeat" means the redis key is missing entirely
// (vs "stale" which means the key exists but is far in the past).
const (
	statusActive          = "active"
	statusLagging         = "lagging"
	statusStale           = "stale"
	statusStaleNoHeartbeat = "stale_no_heartbeat"
)

func init() {
	register(defs.GetTargetRealtimeStatus, getTargetRealtimeStatus)
	register(defs.QueryHostMetricsWindow, queryHostMetricsWindow)
	register(defs.ListNeighborTargets, listNeighborTargets)
}

// classifyLag maps "seconds since last beat" to a status string. Pulled out so
// get_target_realtime_status and list_neighbor_targets stay in sync.
func classifyLag(lag int64) string {
	switch {
	case lag < 60:
		return statusActive
	case lag < 180:
		return statusLagging
	default:
		return statusStale
	}
}

// =============================================================================
// get_target_realtime_status
// =============================================================================

type realtimeStatusResult struct {
	Ident        string                 `json:"ident"`
	BeatTime     int64                  `json:"beat_time"`
	BeatTimeStr  string                 `json:"beat_time_str,omitempty"`
	LagSeconds   int64                  `json:"lag_seconds"`
	Status       string                 `json:"status"`
	UpdateAtDB   int64                  `json:"update_at_db"`
	Offset       int64                  `json:"offset,omitempty"`
	CpuUtil      float64                `json:"cpu_util,omitempty"`
	MemUtil      float64                `json:"mem_util,omitempty"`
	CpuNum       int                    `json:"cpu_num,omitempty"`
	AgentVersion string                 `json:"agent_version,omitempty"`
	RemoteAddr   string                 `json:"remote_addr,omitempty"`
	OS           string                 `json:"os,omitempty"`
	HostIp       string                 `json:"host_ip,omitempty"`
	ExtendInfo   map[string]interface{} `json:"extend_info,omitempty"`
	GroupIds     []int64                `json:"group_ids,omitempty"`
}

func getTargetRealtimeStatus(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	ident := strings.TrimSpace(getArgString(args, "ident"))
	if ident == "" {
		return "", fmt.Errorf("ident is required")
	}

	target, err := models.TargetGet(deps.DBCtx, "ident=?", ident)
	if err != nil {
		return "", fmt.Errorf("failed to get target: %v", err)
	}
	if target == nil {
		return `{"error":"target not found"}`, nil
	}

	groupIds, _ := models.TargetGroupIdsGetByIdent(deps.DBCtx, ident)
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SlicesOverlap(bgids, groupIds) {
			return "", fmt.Errorf("forbidden: no access to this target")
		}
	}

	result := realtimeStatusResult{
		Ident:        ident,
		UpdateAtDB:   target.UpdateAt,
		AgentVersion: target.AgentVersion,
		OS:           target.OS,
		HostIp:       target.HostIp,
		GroupIds:     groupIds,
		Status:       statusStaleNoHeartbeat,
	}

	beat, meta := readRedisHostState(deps.Redis, ident)
	if beat > 0 {
		result.BeatTime = beat
		result.BeatTimeStr = formatUnixTime(beat)
		result.LagSeconds = time.Now().Unix() - beat
		if result.LagSeconds < 0 {
			result.LagSeconds = 0
		}
		result.Status = classifyLag(result.LagSeconds)
	}
	if meta != nil {
		result.Offset = meta.Offset
		result.CpuUtil = meta.CpuUtil
		result.MemUtil = meta.MemUtil
		result.CpuNum = meta.CpuNum
		if result.RemoteAddr == "" {
			result.RemoteAddr = meta.RemoteAddr
		}
		if meta.AgentVersion != "" {
			result.AgentVersion = meta.AgentVersion
		}
		if meta.HostIp != "" && result.HostIp == "" {
			result.HostIp = meta.HostIp
		}
		result.ExtendInfo = meta.ExtendInfo
	}

	bytes, _ := json.Marshal(result)
	logger.Debugf("get_target_realtime_status: ident=%s lag=%ds status=%s", ident, result.LagSeconds, result.Status)
	return string(bytes), nil
}

// readRedisHostState fetches BeatTime (via n9e_meta_update_time_<ident>) and
// HostMeta (via n9e_meta_<ident>) in two MGet round-trips. Returns (0, nil) if
// redis is unavailable — callers fall back to status=stale_no_heartbeat.
func readRedisHostState(redis storage.Redis, ident string) (int64, *models.HostMeta) {
	if redis == nil {
		return 0, nil
	}
	ctx := context.Background()

	var beat int64
	vals := storage.MGet(ctx, redis, []string{models.WrapIdentUpdateTime(ident)})
	for _, v := range vals {
		if v == nil {
			continue
		}
		var hut models.HostUpdateTime
		if err := json.Unmarshal(v, &hut); err == nil {
			beat = hut.UpdateTime
		}
	}

	var meta *models.HostMeta
	metaVals := storage.MGet(ctx, redis, []string{models.WrapIdent(ident)})
	for _, v := range metaVals {
		if v == nil {
			continue
		}
		var m models.HostMeta
		if err := json.Unmarshal(v, &m); err == nil {
			meta = &m
		}
	}
	return beat, meta
}

// =============================================================================
// query_host_metrics_window
// =============================================================================

var defaultHostMetrics = []string{
	"cpu_usage_active",
	"mem_available_percent",
	"system_load1",
	"net_bytes_recv",
	"net_bytes_sent",
}

type metricAggregate struct {
	Metric       string  `json:"metric"`
	SamplesCount int     `json:"samples_count"`
	FirstTs      int64   `json:"first_ts,omitempty"`
	LastTs       int64   `json:"last_ts,omitempty"`
	Min          float64 `json:"min,omitempty"`
	Max          float64 `json:"max,omitempty"`
	Avg          float64 `json:"avg,omitempty"`
	Last         float64 `json:"last,omitempty"`
	Series       int     `json:"series,omitempty"`
	Error        string  `json:"error,omitempty"`
}

func queryHostMetricsWindow(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	ident := strings.TrimSpace(getArgString(args, "ident"))
	if ident == "" {
		return "", fmt.Errorf("ident is required")
	}

	// Permission check — same gating as get_target_realtime_status.
	groupIds, _ := models.TargetGroupIdsGetByIdent(deps.DBCtx, ident)
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SlicesOverlap(bgids, groupIds) {
			return "", fmt.Errorf("forbidden: no access to this target")
		}
	}

	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId == 0 {
		return "", fmt.Errorf("datasource_id is required (call list_datasources to find a Prometheus datasource id, or pass it explicitly)")
	}

	client := deps.GetPromClient(dsId)
	if client == nil {
		return "", fmt.Errorf("prometheus datasource not found: %d", dsId)
	}

	timeRange := getArgString(args, "time_range")
	if timeRange == "" {
		timeRange = "10m"
	}
	stime, etime := parseTimeRange(timeRange)
	if stime == 0 {
		now := time.Now()
		etime = now.Unix()
		stime = now.Add(-10 * time.Minute).Unix()
	}

	metricsArg := getArgString(args, "metrics")
	metrics := splitMetricList(metricsArg)
	if len(metrics) == 0 {
		metrics = defaultHostMetrics
	}

	step := autoStep(etime - stime)
	r := prom.Range{
		Start: time.Unix(stime, 0),
		End:   time.Unix(etime, 0),
		Step:  time.Duration(step) * time.Second,
	}

	results := make([]*metricAggregate, len(metrics))
	var wg sync.WaitGroup
	for i, m := range metrics {
		i, m := i, m
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = aggregateMetric(ctx, client, ident, m, r)
		}()
	}
	wg.Wait()

	payload := map[string]interface{}{
		"ident":         ident,
		"datasource_id": dsId,
		"start":         formatUnixTime(stime),
		"end":           formatUnixTime(etime),
		"step":          step,
		"items":         results,
	}
	bytes, _ := json.Marshal(payload)
	return string(bytes), nil
}

func splitMetricList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Accept both space- and comma-separated lists; LLM is inconsistent.
	repl := strings.NewReplacer(",", " ", "\t", " ", "\n", " ")
	fields := strings.Fields(repl.Replace(s))
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, f := range fields {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

func aggregateMetric(ctx context.Context, client prom.API, ident, metric string, r prom.Range) *metricAggregate {
	agg := &metricAggregate{Metric: metric}
	query := fmt.Sprintf(`%s{ident="%s"}`, metric, escapePromLabel(ident))
	value, _, err := client.QueryRange(ctx, query, r)
	if err != nil {
		agg.Error = err.Error()
		return agg
	}
	matrix, ok := value.(model.Matrix)
	if !ok {
		agg.Error = fmt.Sprintf("unexpected result type %s", value.Type().String())
		return agg
	}
	if len(matrix) == 0 {
		return agg
	}
	agg.Series = len(matrix)

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	var sum float64
	var n int
	var lastVal float64
	var firstTs, lastTs int64

	for _, ss := range matrix {
		for _, pt := range ss.Values {
			v := float64(pt.Value)
			if math.IsNaN(v) {
				continue
			}
			ts := int64(pt.Timestamp.Unix())
			if n == 0 {
				firstTs = ts
				lastTs = ts
				lastVal = v
			}
			if ts < firstTs {
				firstTs = ts
			}
			if ts > lastTs {
				lastTs = ts
				lastVal = v
			}
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
			sum += v
			n++
		}
	}
	if n == 0 {
		return agg
	}
	agg.SamplesCount = n
	agg.FirstTs = firstTs
	agg.LastTs = lastTs
	agg.Min = round2(minV)
	agg.Max = round2(maxV)
	agg.Avg = round2(sum / float64(n))
	agg.Last = round2(lastVal)
	return agg
}

func escapePromLabel(s string) string {
	// Prom label values are double-quoted; escape backslashes and quotes.
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return r.Replace(s)
}

func round2(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return math.Round(f*100) / 100
}

// =============================================================================
// list_neighbor_targets
// =============================================================================

type neighborItem struct {
	Ident      string `json:"ident"`
	HostIp     string `json:"host_ip,omitempty"`
	BeatTime   int64  `json:"beat_time,omitempty"`
	LagSeconds int64  `json:"lag_seconds"`
	Status     string `json:"status"`
}

type neighborSummary struct {
	Total   int `json:"total"`
	Active  int `json:"active"`
	Lagging int `json:"lagging"`
	Stale   int `json:"stale"`
}

func listNeighborTargets(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	ident := strings.TrimSpace(getArgString(args, "ident"))
	if ident == "" {
		return "", fmt.Errorf("ident is required")
	}

	groupIds, err := models.TargetGroupIdsGetByIdent(deps.DBCtx, ident)
	if err != nil {
		return "", fmt.Errorf("failed to get target groups: %v", err)
	}
	if len(groupIds) == 0 {
		return `{"items":[],"summary":{"total":0,"active":0,"lagging":0,"stale":0},"note":"target has no busi group"}`, nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SlicesOverlap(bgids, groupIds) {
			return "", fmt.Errorf("forbidden: no access to this target")
		}
	}

	limit := getArgInt(args, "limit", 30)
	if limit > 100 {
		limit = 100
	}
	// Pull one extra so we can drop the self-ident without truncating the page.
	targets, err := models.TargetGets(deps.DBCtx, limit+1, 0, "ident", false,
		models.BuildTargetWhereWithBgids(groupIds))
	if err != nil {
		return "", fmt.Errorf("failed to query neighbor targets: %v", err)
	}

	filtered := make([]*models.Target, 0, len(targets))
	for _, t := range targets {
		if t.Ident == ident {
			continue
		}
		filtered = append(filtered, t)
		if len(filtered) >= limit {
			break
		}
	}
	models.FillTargetsBeatTime(deps.Redis, filtered)

	now := time.Now().Unix()
	items := make([]neighborItem, 0, len(filtered))
	var summary neighborSummary
	for _, t := range filtered {
		item := neighborItem{
			Ident:    t.Ident,
			HostIp:   t.HostIp,
			BeatTime: t.BeatTime,
		}
		if t.BeatTime > 0 {
			item.LagSeconds = now - t.BeatTime
			if item.LagSeconds < 0 {
				item.LagSeconds = 0
			}
			item.Status = classifyLag(item.LagSeconds)
		} else {
			item.Status = statusStaleNoHeartbeat
		}
		switch item.Status {
		case statusActive:
			summary.Active++
		case statusLagging:
			summary.Lagging++
		default:
			summary.Stale++
		}
		items = append(items, item)
	}
	summary.Total = len(items)

	// Sort: stale first, then lagging, then active — surfaces problems on top.
	sort.SliceStable(items, func(i, j int) bool {
		return statusRank(items[i].Status) > statusRank(items[j].Status)
	})

	payload := map[string]interface{}{
		"ident":     ident,
		"group_ids": groupIds,
		"items":     items,
		"summary":   summary,
	}
	bytes, _ := json.Marshal(payload)
	return string(bytes), nil
}

func statusRank(s string) int {
	switch s {
	case statusStaleNoHeartbeat:
		return 3
	case statusStale:
		return 2
	case statusLagging:
		return 1
	default:
		return 0
	}
}
