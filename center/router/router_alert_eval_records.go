package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

const (
	// evalRecordsFanout 单次请求同时在途的**查询任务**数上限。
	//
	// 计量单位是 (datasource_id, 实例) 二元组而不是实例：targets 的去重键是
	// `dsId@node`，转发也是一个 datasource 发一次请求，所以同一个引擎进程负责这条规则的
	// 几个 datasource，就会占用几个槽位——极端情况下这 8 个可以全落在同一个进程上。
	//
	// 它同时也是内存扇出系数：并发中的每个任务最坏持有一份 evalRecordsMaxRespBytes
	// 量级的解码结果，单次请求的峰值堆约等于本值 × evalRecordsMaxRespBytes。
	evalRecordsFanout = 8
	// evalRecordsPerNodeFanout 单次请求打向**同一个引擎实例**的并发上限。
	//
	// 与 evalRecordsLocalFanout 同理，只是管的是远端：引擎侧的并发闸默认
	// MaxConcurrentQueries=2，center 若照 evalRecordsFanout 对着同一个远端实例压 8 个请求，
	// 超出闸值的那几路只会排队到 queryAcquireTimeout(2s) 再拿一个 ErrBusy，用户看到的是
	// 半屏「引擎忙」，而这纯粹是 center 自己造成的。本机路径当初已经对齐过，远端漏了。
	// 取引擎默认值 2；远端配得更高时，单次请求只是不把它压满，不影响正确性。
	evalRecordsPerNodeFanout = 2
	// evalRecordsMaxAggregations 本进程同时执行的**聚合查询**数上限（跨请求）。
	//
	// evalRecordsFanout 只约束单次请求，多个用户同时刷新抽屉时峰值堆仍是它的整数倍：
	// 该 handler 只有 auth+user+perm("/alert-rules") 一道权限门，具备该权限的普通用户
	// 反复刷新即可让 center 的堆成倍放大。与引擎侧 MaxConcurrentQueries 取同一个数量级，
	// 排队等待也复用同样的语义（排到超时就给明确的可重试错误，而不是含糊的客户端超时）。
	evalRecordsMaxAggregations = 2
	// evalRecordsAcquireTimeout 聚合闸的排队等待上限，与引擎侧 queryAcquireTimeout 对齐。
	evalRecordsAcquireTimeout = 2 * time.Second
	// evalRecordsMaxRespBytes 单个节点响应体的读取上限；超出即报错而不是静默截断。
	// 必须大于引擎侧的 MaxQueryBytes（默认 32MB），否则正常查询会被这里判成超限；
	// 留 1.5 倍余量给 JSON 外层与字段名开销。
	evalRecordsMaxRespBytes = 48 * 1024 * 1024
	// evalRecordsEngineAliveSec 判定引擎实例仍在心跳的时间窗，与 naming.ActiveServers 一致
	evalRecordsEngineAliveSec = 30
	// evalRecordsExtraTargets 「历史 owner 兜底」额外查询的实例数预算。
	// 每个 datasource 的当前 owner 一律纳入、不受此限；只有兜底这部分会被裁，
	// 免得它把主路径的扇出撑爆（扇出并发只有 evalRecordsFanout，每个还带 5s 超时）。
	evalRecordsExtraTargets = 32
	// evalRecordsNodeTimeout 单个节点的转发超时：edge 节点可能被防火墙 drop，长超时会拖住整个请求
	evalRecordsNodeTimeout = 5 * time.Second
	// evalRecordsTotalTimeout 整个扇出的总时限。
	//
	// 没有它，耗时上限是「目标数 ÷ 并发 × 单节点超时」：一条按 cate 匹配到几十个数据源的
	// 规则，遇上一批不可达的 edge 节点就是几十秒——这期间 center 侧一直占着这个请求的
	// goroutine 与已收到的结果内存，用户则对着转圈的抽屉毫无信息。超时后剩余节点直接记为
	// 失败并透出，已查到的记录照常返回。
	evalRecordsTotalTimeout = 15 * time.Second
)

// evalRecordsClient 转发用的共享客户端。
// Timeout 与请求 context 双保险：前者兜住整次请求（含读响应体），后者让扇出总时限一到
// 就立刻掐断在途连接，不必再等满单节点超时。
var evalRecordsClient = &http.Client{Timeout: evalRecordsNodeTimeout}

// evalRecordsAggSem 跨请求的聚合并发闸，见 evalRecordsMaxAggregations。
var evalRecordsAggSem = make(chan struct{}, evalRecordsMaxAggregations)

// acquireEvalRecordsAgg 取一个聚合令牌，排队超过 evalRecordsAcquireTimeout 则放弃。
// 不做「拿不到就立刻拒」：单次聚合通常几十毫秒，几个用户同时点开抽屉属于常态，排一下队就过去了。
func acquireEvalRecordsAgg(ctx context.Context) bool {
	select {
	case evalRecordsAggSem <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(evalRecordsAcquireTimeout)
	defer timer.Stop()
	select {
	case evalRecordsAggSem <- struct{}{}:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// alertRuleEvalRecords 查询告警规则的评估执行记录。
// GET /api/n9e/alert-rule/:arid/eval-records?datasource_id=&from=&to=&before=&limit=
// from/to 为 unix 秒（默认最近 1 小时），before 为毫秒游标，结果按 ts 倒序。
// 记录存储在规则所属引擎节点的本地磁盘，按 hashring 定位节点，本机命中则直接读，否则转发。
func (rt *Router) alertRuleEvalRecords(c *gin.Context) {
	ruleId := ginx.UrlParamInt64(c, "arid")
	rule, err := models.AlertRuleGetById(rt.Ctx, ruleId)
	ginx.Dangerous(err)
	if rule == nil {
		ginx.Bomb(200, "no such alert rule")
	}

	// 业务组权限校验：非管理员需可读该规则所属业务组
	rt.bgroCheck(c, rule.GroupId)

	// 跨请求聚合闸：放在权限校验之后、扇出与心跳查库之前，让所有吃内存/吃 DB 的动作都在闸内。
	// 拿不到就给一个明确的可重试错误，而不是让它去和别的请求抢堆——空结果会被读成"当时没有记录"。
	if !acquireEvalRecordsAgg(c.Request.Context()) {
		ginx.Bomb(200, "too many concurrent eval-record queries on this center instance, please retry")
	}
	defer func() { <-evalRecordsAggSem }()

	now := time.Now().Unix()
	from := ginx.QueryInt64(c, "from", now-3600)
	to := ginx.QueryInt64(c, "to", now)
	before := ginx.QueryInt64(c, "before", 0)
	limit := int(ginx.QueryInt64(c, "limit", int64(evallog.DefaultQueryLimit)))
	if limit <= 0 || limit > evallog.MaxQueryLimit {
		limit = evallog.DefaultQueryLimit
	}
	queryDsId := ginx.QueryInt64(c, "datasource_id", 0)

	// 确定要查询的 datasource 集合：host 类规则的 worker 以 datasourceId=0 运行
	var dsIds []int64
	if queryDsId > 0 {
		dsIds = []int64{queryDsId}
	} else if rule.IsHostRule() {
		dsIds = []int64{0}
	} else {
		dsIds = rt.DatasourceCache.GetIDsByDsCateAndQueries(rule.Cate, rule.DatasourceQueries)
		if len(dsIds) == 0 {
			dsIds = []int64{0}
		}
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)
	ruleIdStr := strconv.FormatInt(ruleId, 10)

	// 解析各 datasource 对应的引擎节点并去重。
	//
	// 不能只查 hashring 当前 owner：记录写在**评估时**所属节点的本地磁盘，而 hashring 的
	// 归属会随实例增删漂移。扩缩容后规则迁到新 owner，保留期内的旧记录仍躺在原节点上却
	// 永远查不到，页面只看到一个空列表，被读成「当时确实没评估过」——正是本功能要消除的
	// 那种歧义。所以在 owner 之外，把该 datasource（host 规则则是该 engine_cluster）下
	// 仍在心跳的其余实例也纳入查询。
	type dsNode struct {
		dsId  int64
		node  string
		owner bool // 当前 hashring 归属；false 表示只是历史 owner 兜底
	}
	var targets []dsNode
	seen := make(map[string]struct{})
	add := func(dsId int64, node string, owner bool) {
		key := fmt.Sprintf("%d@%s", dsId, node)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		targets = append(targets, dsNode{dsId: dsId, node: node, owner: owner})
	}

	// 存活实例只取这一次，owner 解析与兜底两轮共用
	peerInstances := rt.evalRecordsPeerInstances(rule, dsIds)

	// 第一轮：每个 datasource 的当前 owner，全部纳入（与只查 owner 时的行为一致）
	owners := make(map[int64]string, len(dsIds))
	for _, dsId := range dsIds {
		var node string
		var err error
		if rule.IsHostRule() || dsId == 0 {
			// host 规则以 EngineName 为 hashring key
			node, err = naming.DatasourceHashRing.GetNode(rt.Alert.Heartbeat.EngineName, ruleIdStr)
		} else {
			node, err = evalRecordsOwner(dsId, ruleIdStr, peerInstances[dsId])
		}
		if err != nil {
			// hashring 未就绪等场景，退化为本机查询
			node = instance
		}
		owners[dsId] = node
		add(dsId, node, true)
	}

	// 第二轮：历史 owner 兜底，受 evalRecordsExtraTargets 预算约束
	extra := 0
peers:
	for _, dsId := range dsIds {
		for _, node := range peerInstances[dsId] {
			if node == owners[dsId] {
				continue
			}
			if _, ok := seen[fmt.Sprintf("%d@%s", dsId, node)]; ok {
				continue
			}
			if extra >= evalRecordsExtraTargets {
				logger.Warningf("eval records: rule %d has more peer engine instances than the %d budget; "+
					"records left on instances beyond it are not included", ruleId, evalRecordsExtraTargets)
				break peers
			}
			extra++
			add(dsId, node, false)
		}
	}

	// 并发查询各目标节点（edge 节点不可达时靠短超时快速失败），
	// 失败节点不静默丢弃，透出给前端以区分"没有记录"和"取不到记录"。
	// 扇出加信号量限流：一条按 cate 匹配的规则可能命中几十个数据源，每个 goroutine
	// 最坏会驻留一份 evalRecordsMaxRespBytes 的响应，不限流内存会成倍放大。
	ctx, cancel := context.WithTimeout(c.Request.Context(), evalRecordsTotalTimeout)
	defer cancel()

	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		merged    []evallog.EvalRecord
		instances []string
		nodeErrs  []evalRecordsNodeErr
		disabled  []string
		truncated bool
		// 兜底实例的失败信息单独收集：正常集群里绝大多数 peer 本来就没存过这条规则的记录，
		// 把它们的「不可达 / 没开 evallog」一律抛到前端，会把原本只有一行的告警条变成 N 行。
		// 只在最终一条记录都没查到时才并入——那时用户正对着空列表发问，peer 取不到才真的
		// 可能是答案；有记录时这些信息只是噪音。
		peerErrs     []evalRecordsNodeErr
		peerDisabled []string
		sem          = make(chan struct{}, evalRecordsFanout)
		localSem     = make(chan struct{}, evalRecordsLocalFanout())
	)

	// 每个远端实例单独一个闸，见 evalRecordsPerNodeFanout。构造完即只读，goroutine 里直接查。
	nodeSems := make(map[string]chan struct{}, len(targets))
	for _, t := range targets {
		if t.node == instance {
			continue // 本机走 localSem，它已经对齐过引擎的并发闸
		}
		if _, ok := nodeSems[t.node]; !ok {
			nodeSems[t.node] = make(chan struct{}, evalRecordsPerNodeFanout)
		}
	}

	for _, t := range targets {
		wg.Add(1)
		go func(t dsNode) {
			defer wg.Done()
			// 远端要过两道闸：先本节点闸、再总扇出闸。
			// 顺序对所有 goroutine 一致才不会死锁；且**先节点后总闸**很关键——反过来的话，
			// 同一个节点的多个任务会占着总闸的令牌去排节点闸，把其余节点的名额全占住。
			gates := []chan struct{}{localSem}
			if t.node != instance {
				gates = []chan struct{}{nodeSems[t.node], sem}
			}
			// 已拿到的必须显式记：没拿到令牌的 goroutine 若也去 <-gate 释放，
			// 放走的是别人的令牌，扇出并发就被悄悄放大了
			held := make([]chan struct{}, 0, len(gates))
			defer func() {
				for i := len(held) - 1; i >= 0; i-- {
					<-held[i]
				}
			}()
			for _, g := range gates {
				select {
				case g <- struct{}{}:
					held = append(held, g)
				case <-ctx.Done():
					// 总时限已到，排在后面的节点不必再发起查询
				}
				if ctx.Err() != nil {
					break
				}
			}

			var res evalRecordsNodeResult
			var err error
			switch {
			case ctx.Err() != nil:
				err = fmt.Errorf("fanout deadline exceeded before querying %s (%v); "+
					"the rule spans too many engine instances, narrow it down with datasource_id", t.node, ctx.Err())
			case t.node == instance:
				var local evallog.QueryResult
				local, err = evallog.QueryRecords(ruleId, t.dsId, from*1000, to*1000, before, limit)
				res = evalRecordsNodeResult{
					records:   local.Records,
					enabled:   err != evallog.ErrNotEnabled,
					truncated: local.Truncated,
					note:      local.Note,
				}
				if err == evallog.ErrNotEnabled {
					err = nil
				}
			default:
				res, err = rt.forwardEvalRecords(ctx, t.node, ruleId, t.dsId, from, to, before, limit)
			}

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				e := evalRecordsNodeErr{
					Instance:     t.node,
					DatasourceId: t.dsId,
					Error:        err.Error(),
				}
				if t.owner {
					nodeErrs = append(nodeErrs, e)
				} else {
					peerErrs = append(peerErrs, e)
				}
				return
			}
			if !res.enabled {
				// 该节点没开 evallog：既不是"没有记录"也不是"取不到记录"，必须单独透出，
				// 否则前端只能看到一个空列表，会被读成"当时确实没有数据"
				e := evalRecordsNodeErr{
					Instance:     t.node,
					DatasourceId: t.dsId,
					Error:        "eval records not enabled on this engine instance ([Alert.EvalLog] Disable = true)",
				}
				if t.owner {
					disabled = append(disabled, t.node)
					nodeErrs = append(nodeErrs, e)
				} else {
					peerDisabled = append(peerDisabled, t.node)
					peerErrs = append(peerErrs, e)
				}
				return
			}
			if res.truncated {
				// 节点侧的字节预算生效了：条数变少不是"就这么多记录"，必须说出来。
				// 走 errors 通道是因为前端已有的提示条正是「以下节点的记录未包含在结果中」，
				// 连同它给出的按节点直查命令一起，恰好是用户此时该做的下一步。
				truncated = true
				nodeErrs = append(nodeErrs, evalRecordsNodeErr{
					Instance:     t.node,
					DatasourceId: t.dsId,
					Error:        res.noteOrDefault(),
				})
			}
			merged = append(merged, res.records...)
			// 边收边收口：不这么做的话，扇出到 N 个节点就会在 center 侧同时驻留 N × limit
			// 条记录（每条最大可到 176KB），最后却只用其中 limit 条。收口后常驻只有 limit 条
			// 加当前这一批
			merged = sortTruncEvalRecords(merged, limit)
			instances = append(instances, t.node)
		}(t)
	}
	wg.Wait()

	merged = sortTruncEvalRecords(merged, limit)
	if merged == nil {
		merged = []evallog.EvalRecord{}
	}

	if len(merged) == 0 {
		nodeErrs = append(nodeErrs, peerErrs...)
		disabled = append(disabled, peerDisabled...)
	}

	// 同一实例可能承载多个 datasource，纳入历史 owner 后重复更明显；这两个列表都是
	// 按实例展示的，去重后前端不必再自己处理
	instances = uniqStrings(instances)
	disabled = uniqStrings(disabled)

	ginx.NewRender(c).Data(gin.H{
		"list":               merged,
		"instances":          instances,
		"errors":             nodeErrs,
		"disabled_instances": disabled,
		// 任一节点触发了字节预算：本页条数少于 limit 也不代表"就这么多记录"
		"truncated": truncated,
	}, nil)
}

// evalRecordsLocalFanout 本机目标的并发上限，对齐引擎侧的并发闸。
//
// center 与 alert 合并部署时（默认形态），一条命中多个数据源的规则，它的 owner 往往全是
// 本机，于是这些目标全走进程内的 evallog.QueryRecords。若照 evalRecordsFanout 并发压进去，
// 超出闸值的那几路只会排队到超时、各拿一个 ErrBusy——用户看到的是半屏"引擎忙"，而这纯粹
// 是自己把自己挤住的。本机查询是本地文件读，没有网络延迟要靠并发掩盖，按闸值对齐即可。
//
// 引擎未启用时 QueryConcurrency() 返回 0，兜到 1：那种情况下本机查询会立刻返回未启用。
func evalRecordsLocalFanout() int {
	return max(1, evallog.QueryConcurrency())
}

// evalRecordsOwner 解析某 datasource 当前的 hashring 归属节点。
//
// 与 getNodeForDatasource 同义，区别只在于 hashring 里没有这个 datasource 时（常见于该
// 数据源属于另一个 engine_cluster），用**已经取回的**存活实例就地建环，而不是再查一次库：
// 按 cate 匹配的规则可能命中几十个数据源，逐个回查就是几十条串行 SQL 压在页面接口上。
func evalRecordsOwner(dsId int64, pk string, alive []string) (string, error) {
	node, err := naming.DatasourceHashRing.GetNode(strconv.FormatInt(dsId, 10), pk)
	if err == nil {
		return node, nil
	}
	if len(alive) == 0 {
		return "", fmt.Errorf("no active instances for datasource %d", dsId)
	}
	return naming.NewConsistentHashRing(int32(naming.NodeReplicas), alive).Get(pk)
}

// evalRecordsNodeResult 单个节点的查询结果。
type evalRecordsNodeResult struct {
	records   []evallog.EvalRecord
	enabled   bool // 该节点是否开启了 evallog
	truncated bool // 该节点因字节预算提前收口
	note      string
}

func (r evalRecordsNodeResult) noteOrDefault() string {
	if r.note != "" {
		return r.note
	}
	return "result truncated by the engine's per-query byte budget, older records in this range are not included; narrow the time range"
}

// sortTruncEvalRecords 按 ts 倒序并只保留前 limit 条。
func sortTruncEvalRecords(recs []evallog.EvalRecord, limit int) []evallog.EvalRecord {
	if len(recs) <= limit {
		// 仍需保证有序：调用方拿到的必须是可直接返回的倒序结果
		sort.Slice(recs, func(i, j int) bool { return recs[i].Ts > recs[j].Ts })
		return recs
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Ts > recs[j].Ts })
	// 截掉的部分显式置零，让被丢弃记录里的曲线数据能立刻被 GC 回收，
	// 而不是被底层数组一直引用到这次请求结束
	for i := limit; i < len(recs); i++ {
		recs[i] = evallog.EvalRecord{}
	}
	return recs[:limit]
}

// uniqStrings 去重并保持出现顺序；返回非 nil 切片，保证序列化成 [] 而不是 null。
func uniqStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// evalRecordsPeerInstances 返回每个 datasource 上仍在心跳的全部引擎实例，
// 用于覆盖 hashring 重新分配之前的历史 owner（记录留在原节点磁盘上，不会跟着迁移）。
// 查询失败时返回 nil：兜底路径不该让主查询失败，当前 owner 已经单独纳入了。
//
// 一次取回全部存活心跳再在内存里分桶，而不是每个 datasource 查一次库：按 cate 匹配的
// 规则可能命中几十个数据源，逐个查就是几十条串行 SQL 压在这个页面接口上。
func (rt *Router) evalRecordsPeerInstances(rule *models.AlertRule, dsIds []int64) map[int64][]string {
	aliveSince := time.Now().Unix() - evalRecordsEngineAliveSec

	engines, err := models.AlertingEngineGetsAlive(rt.Ctx, aliveSince)
	if err != nil {
		logger.Warningf("eval records: list alive engine instances for rule %d error: %v", rule.Id, err)
		return nil
	}

	out := make(map[int64][]string, len(dsIds))
	for _, dsId := range dsIds {
		seen := make(map[string]struct{}, len(engines))
		for _, e := range engines {
			// host 规则的 worker 以 datasourceId=0 运行、按 engine_cluster 组成 hashring；
			// 同一实例在这张表里每个 datasource 一行，cluster 分支下必然重复，要去重
			if rule.IsHostRule() || dsId == 0 {
				if e.EngineCluster != rt.Alert.Heartbeat.EngineName {
					continue
				}
			} else if e.DatasourceId != dsId {
				continue
			}
			if _, ok := seen[e.Instance]; ok {
				continue
			}
			seen[e.Instance] = struct{}{}
			out[dsId] = append(out[dsId], e.Instance)
		}
	}
	return out
}

// evalRecordsNodeErr 单个引擎节点查询失败信息。前端据此提示：
// 可登录该节点本机访问 /v1/n9e/eval-records 接口查看记录。
type evalRecordsNodeErr struct {
	Instance     string `json:"instance"`
	DatasourceId int64  `json:"datasource_id"`
	Error        string `json:"error"`
}

// forwardEvalRecords 向目标引擎节点转发查询。
func (rt *Router) forwardEvalRecords(ctx context.Context, node string, ruleId, dsId, from, to, before int64, limit int) (evalRecordsNodeResult, error) {
	var zero evalRecordsNodeResult

	url := fmt.Sprintf("http://%s/v1/n9e/eval-records?rule_id=%d&datasource_id=%d&from=%d&to=%d&before=%d&limit=%d",
		node, ruleId, dsId, from, to, before, limit)
	// 挂上扇出的总时限：整体已经超时了就不该再为某个慢节点单独等满 5s
	reqCtx, cancel := context.WithTimeout(ctx, evalRecordsNodeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return zero, err
	}

	for user, pass := range rt.HTTP.APIForService.BasicAuth {
		req.SetBasicAuth(user, pass)
		break
	}

	resp, err := evalRecordsClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("forward to %s failed: %v", node, err)
	}
	defer resp.Body.Close()

	// 边读边解，不再先 io.ReadAll 出一份完整响应体：一个节点最大可返回几十 MB，
	// 而扇出并发是 evalRecordsFanout，「原始字节 + 解码结果」同时驻留就是双倍的峰值堆。
	// 多给 1 字节的额度用于判断是否触到上限——LimitReader 截断时不报错，
	// 直接解码只会得到"JSON 语法错误"这种看不出根因的信息。
	counter := &countingReader{r: io.LimitReader(resp.Body, evalRecordsMaxRespBytes+1)}
	overSize := func() error {
		return fmt.Errorf("response from %s exceeds %d bytes, narrow the time range or lower limit",
			node, evalRecordsMaxRespBytes)
	}

	var result struct {
		Dat struct {
			List      []evallog.EvalRecord `json:"list"`
			Enabled   *bool                `json:"enabled"`
			Truncated bool                 `json:"truncated"`
			Note      string               `json:"note"`
		} `json:"dat"`
		Err string `json:"err"`
	}
	if err := json.NewDecoder(counter).Decode(&result); err != nil {
		if counter.n > evalRecordsMaxRespBytes {
			return zero, overSize()
		}
		return zero, err
	}
	if counter.n > evalRecordsMaxRespBytes {
		return zero, overSize()
	}
	if result.Err != "" {
		return zero, fmt.Errorf("%s", result.Err)
	}
	return evalRecordsNodeResult{
		records: result.Dat.List,
		// 老版本引擎不返回 enabled 字段，缺省按启用处理，保持向后兼容
		enabled:   result.Dat.Enabled == nil || *result.Dat.Enabled,
		truncated: result.Dat.Truncated,
		note:      result.Dat.Note,
	}, nil
}

// countingReader 记录已读字节数，用于区分"响应超限"与"响应本身是坏 JSON"。
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
