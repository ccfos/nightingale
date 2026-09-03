package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

// probe_target_onboard_status
//
// 与 get_target_realtime_status 的关键差异：本工具**容忍 target 不在 DB 中**。
// 适用场景是"机器没出现 / OS 显示 unknown / 心跳没建立"——target row 经常根本就
// 没落库，老 host_health 系列直接 return "target not found" 就断了。
//
// 输出沿"接入链路 5 段"组织足迹，likely_segment 用最深一段有证据的位置定位断点。

func init() {
	register(defs.ProbeTargetOnboardStatus, probeTargetOnboardStatus)
}

const (
	segmentCategrafOrHTTP = "segment_1_or_2" // categraf 本机进程 / HTTP 上报
	segmentServerRecv     = "segment_3"      // server 接收但 heartbeat 元数据没落
	segmentTargetDB       = "segment_4"      // target 表落库但 redis 没数据
	segmentPromIngest     = "segment_5"      // redis 有 beat 但 prom 没指标
	segmentOK             = "ok"
)

type onboardTargetSnap struct {
	Id           int64   `json:"id"`
	OS           string  `json:"os"`
	HostIp       string  `json:"host_ip,omitempty"`
	AgentVersion string  `json:"agent_version,omitempty"`
	UpdateAt     int64   `json:"update_at"`
	UpdateAtStr  string  `json:"update_at_str,omitempty"`
	GroupIds     []int64 `json:"group_ids"`
	HasGroup     bool    `json:"has_group"`
}

type onboardRedisSnap struct {
	CpuUtil      float64 `json:"cpu_util,omitempty"`
	MemUtil      float64 `json:"mem_util,omitempty"`
	RemoteAddr   string  `json:"remote_addr,omitempty"`
	AgentVersion string  `json:"agent_version,omitempty"`
	OS           string  `json:"os,omitempty"`
	Hostname     string  `json:"hostname,omitempty"`
	Offset       int64   `json:"offset,omitempty"`
}

type onboardProbeResult struct {
	Ident string `json:"ident"`

	// 段 3/4：DB
	InTargetDb bool               `json:"in_target_db"`
	Target     *onboardTargetSnap `json:"target,omitempty"`

	// 段 4：Redis
	InRedisBeat      bool              `json:"in_redis_beat"`
	InRedisMeta      bool              `json:"in_redis_meta"`
	RedisBeatTime    int64             `json:"redis_beat_time,omitempty"`
	RedisBeatTimeStr string            `json:"redis_beat_time_str,omitempty"`
	LagSeconds       int64             `json:"lag_seconds,omitempty"`
	HeartbeatStatus  string            `json:"heartbeat_status,omitempty"`
	RedisMeta        *onboardRedisSnap `json:"redis_meta,omitempty"`

	// 段 5：Prom
	DatasourceId      int64    `json:"datasource_id,omitempty"`
	DatasourceChecked bool     `json:"datasource_checked"`
	InPromTargetUp    bool     `json:"in_prom_target_up"`
	TargetUpLast      *float64 `json:"target_up_last,omitempty"`
	PromMetricsHit    int      `json:"prom_metrics_hit"`

	// 诊断提示（工具层预聚合，避免 LLM 自己反推链路位置时翻车）
	LikelySegment string   `json:"likely_segment"`
	LikelyCauses  []string `json:"likely_causes,omitempty"`
}

func probeTargetOnboardStatus(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	ident := strings.TrimSpace(getArgString(args, "ident"))
	if ident == "" {
		return "", fmt.Errorf("ident is required")
	}

	res := &onboardProbeResult{Ident: ident}

	// === 段 3/4：DB ===
	target, err := models.TargetGet(deps.DBCtx, "ident=?", ident)
	if err != nil {
		return "", fmt.Errorf("failed to query target: %v", err)
	}

	var groupIds []int64
	if target != nil {
		res.InTargetDb = true
		groupIds, _ = models.TargetGroupIdsGetByIdent(deps.DBCtx, ident)
		res.Target = &onboardTargetSnap{
			Id:           target.Id,
			OS:           target.OS,
			HostIp:       target.HostIp,
			AgentVersion: target.AgentVersion,
			UpdateAt:     target.UpdateAt,
			UpdateAtStr:  formatUnixTime(target.UpdateAt),
			GroupIds:     groupIds,
			HasGroup:     len(groupIds) > 0,
		}
	}

	// 权限：与 host_health 不同——允许 target 不在 DB / 未归组的查询，否则用户排查
	// "机器没出现"时会被自己的权限锁死。target 在 DB 且有组时，仍按组交集检查。
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if target != nil && len(groupIds) > 0 && !int64SlicesOverlap(bgids, groupIds) {
			return "", fmt.Errorf("forbidden: no access to this target")
		}
	}

	// === 段 4：Redis 心跳 + meta（复用 host_health.go 的 readRedisHostState）===
	beat, meta := readRedisHostState(deps.Redis, ident)
	if beat > 0 {
		res.InRedisBeat = true
		res.RedisBeatTime = beat
		res.RedisBeatTimeStr = formatUnixTime(beat)
		lag := time.Now().Unix() - beat
		if lag < 0 {
			lag = 0
		}
		res.LagSeconds = lag
		res.HeartbeatStatus = classifyLag(lag)
	}
	if meta != nil {
		res.InRedisMeta = true
		res.RedisMeta = &onboardRedisSnap{
			CpuUtil:      meta.CpuUtil,
			MemUtil:      meta.MemUtil,
			RemoteAddr:   meta.RemoteAddr,
			AgentVersion: meta.AgentVersion,
			OS:           meta.OS,
			Hostname:     meta.Hostname,
			Offset:       meta.Offset,
		}
	}

	// === 段 5：Prom（可选，没数据源也不 fatal）===
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId > 0 && deps.GetPromClient != nil {
		if client := deps.GetPromClient(dsId); client != nil {
			res.DatasourceId = dsId
			res.DatasourceChecked = true
			now := time.Now()
			esc := escapePromLabel(ident)
			// target_up 是心跳到 prom 的最直接信号。
			if v, ok := promInstantFirstSample(ctx, client, fmt.Sprintf(`target_up{ident="%s"}`, esc), now); ok {
				res.InPromTargetUp = true
				vv := round2(v)
				res.TargetUpLast = &vv
				res.PromMetricsHit++
			}
			// system_load1 兜底：心跳挂着但用户不上报 target_up 的奇异情形也能识别。
			if _, ok := promInstantFirstSample(ctx, client, fmt.Sprintf(`system_load1{ident="%s"}`, esc), now); ok {
				res.PromMetricsHit++
			}
		}
	}

	res.LikelySegment, res.LikelyCauses = diagnoseOnboardSegment(res)

	bytes, _ := json.Marshal(res)
	logger.Debugf("probe_target_onboard_status: ident=%s segment=%s in_db=%v in_redis=%v prom_hit=%d",
		ident, res.LikelySegment, res.InTargetDb, res.InRedisBeat, res.PromMetricsHit)
	return string(bytes), nil
}

// promInstantFirstSample 做一次 instant 查询，命中至少一个非 NaN 样本时返回 (value, true)。
// 失败 / 空结果 / NaN 一律 (0, false)——本工具是"能不能看到这台机器"的探针，不需要把错误抛上去。
func promInstantFirstSample(ctx context.Context, client prom.API, query string, ts time.Time) (float64, bool) {
	value, _, err := client.Query(ctx, query, ts)
	if err != nil || value == nil {
		return 0, false
	}
	vec, ok := value.(model.Vector)
	if !ok || len(vec) == 0 {
		return 0, false
	}
	for _, s := range vec {
		v := float64(s.Value)
		if !math.IsNaN(v) {
			return v, true
		}
	}
	return 0, false
}

// diagnoseOnboardSegment 把链路足迹折叠成"最深一段有证据的位置"。规则按段位倒序判断，
// 一旦命中就返回——这避免了 LLM 自己反推时把"DB 有 + redis 也有"误判成段 3 问题。
func diagnoseOnboardSegment(r *onboardProbeResult) (string, []string) {
	switch {
	case r.InRedisBeat && r.PromMetricsHit > 0 && r.InTargetDb:
		return segmentOK, nil

	case r.InRedisBeat && r.PromMetricsHit == 0 && r.DatasourceChecked:
		// 心跳到了但 prom 没指标：写入端 / 多集群 / omit_hostname
		return segmentPromIngest, []string{
			"categraf [[writers]] 未配置或 url 错（参考 #2574 #2885）",
			"omit_hostname=true 导致 ident 标签丢失（参考 #1609）",
			"多集群部署时 categraf 与 server 走的数据源不一致，redis 注册到了中心但时序写到了别处（参考 #989）",
			"ident 含特殊字符（() / [] / *）导致 PromQL ident=\"...\" 命中失败（参考 #2052 #2163 #1396）",
		}

	case r.InTargetDb && !r.InRedisBeat:
		// target 落库但 redis 没数据：edge redis 配置 / 版本不一致 / token
		return segmentTargetDB, []string{
			"n9e-edge 没配 redis 或 redis 写失败（参考 #1888）",
			"n9e / n9e-edge 版本不一致，导致 heartbeat 路径不同（参考 #2834 #2764）",
			"Helm/k8s 多副本部署时 APIForService/APIForAgent token 配置异常（参考 #1730 #2177）",
			"redis maxmemory-policy 把心跳 key 驱逐了（参考 #1888 评论）",
		}

	case r.InTargetDb && r.Target != nil && (r.Target.OS == "" || r.Target.OS == "unknown" || r.Target.AgentVersion == ""):
		// target 有但元数据缺：categraf 没开 heartbeat
		return segmentServerRecv, []string{
			"categraf config.toml 的 [heartbeat] enable 未设为 true（参考 #1996 #1667 #1435）",
			"categraf config.toml 的 omit_hostname=true 导致 hostname 字段未上报（参考 #1609）",
			"categraf 版本过老（< v0.2.35）heartbeat 接口对不上（参考 #1434 #1078）",
			"identity 计算 shell 失败导致 ident 字段空（参考 #39 #359 #471 #253 #663）",
		}

	case !r.InTargetDb && !r.InRedisBeat:
		// 哪里都没找到：根本没接通
		return segmentCategrafOrHTTP, []string{
			"categraf 进程没在跑 / 配置里 [heartbeat] enable=false（参考 #1996）",
			"categraf 连不上 n9e：网络/防火墙/端口（参考 #1543 #1492）",
			"TLS 证书 unknown authority / 自签证书未配 ca_file（参考 #2772 #2574）",
			"hostname 与已有机器重名，n9e 端拒绝写入（参考 #2536 #1964）",
			"BasicAuth / token 不匹配（参考 #524 #1808）",
			"Windows 系统不支持 / agent 版本不对（参考 #780 #2520）",
		}

	case r.InTargetDb && r.Target != nil && !r.Target.HasGroup:
		// 已接入但没归组：分类成 OK 但额外提示
		return segmentOK, []string{
			"机器已接通但未归入任何业务组——非接入问题，但会被订阅规则 / 业务组过滤的告警漏掉（参考 #416）",
		}

	default:
		return segmentOK, nil
	}
}
