package router

import (
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
	// evalRecordsFanout 同时查询的引擎节点数上限
	evalRecordsFanout = 8
	// evalRecordsMaxRespBytes 单个节点响应体的读取上限；超出即报错而不是静默截断
	evalRecordsMaxRespBytes = 64 * 1024 * 1024
	// evalRecordsEngineAliveSec 判定引擎实例仍在心跳的时间窗，与 naming.ActiveServers 一致
	evalRecordsEngineAliveSec = 30
	// evalRecordsExtraTargets 「历史 owner 兜底」额外查询的实例数预算。
	// 每个 datasource 的当前 owner 一律纳入、不受此限；只有兜底这部分会被裁，
	// 免得它把主路径的扇出撑爆（扇出并发只有 evalRecordsFanout，每个还带 5s 超时）。
	evalRecordsExtraTargets = 32
)

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

	// 第一轮：每个 datasource 的当前 owner，全部纳入（与只查 owner 时的行为一致）
	owners := make(map[int64]string, len(dsIds))
	for _, dsId := range dsIds {
		var node string
		var err error
		if rule.IsHostRule() || dsId == 0 {
			// host 规则以 EngineName 为 hashring key
			node, err = naming.DatasourceHashRing.GetNode(rt.Alert.Heartbeat.EngineName, ruleIdStr)
		} else {
			node, err = rt.getNodeForDatasource(dsId, ruleIdStr)
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
		for _, node := range rt.evalRecordsPeerInstances(rule, dsId) {
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
	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		merged    []evallog.EvalRecord
		instances []string
		nodeErrs  []evalRecordsNodeErr
		disabled  []string
		// 兜底实例的失败信息单独收集：正常集群里绝大多数 peer 本来就没存过这条规则的记录，
		// 把它们的「不可达 / 没开 evallog」一律抛到前端，会把原本只有一行的告警条变成 N 行。
		// 只在最终一条记录都没查到时才并入——那时用户正对着空列表发问，peer 取不到才真的
		// 可能是答案；有记录时这些信息只是噪音。
		peerErrs     []evalRecordsNodeErr
		peerDisabled []string
		sem          = make(chan struct{}, evalRecordsFanout)
	)
	for _, t := range targets {
		wg.Add(1)
		go func(t dsNode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var recs []evallog.EvalRecord
			var enabled bool
			var err error
			if t.node == instance {
				recs, err = evallog.QueryRecords(ruleId, t.dsId, from*1000, to*1000, before, limit)
				enabled = err != evallog.ErrNotEnabled
				if err == evallog.ErrNotEnabled {
					err = nil
				}
			} else {
				recs, enabled, err = rt.forwardEvalRecords(t.node, ruleId, t.dsId, from, to, before, limit)
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
			if !enabled {
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
			merged = append(merged, recs...)
			instances = append(instances, t.node)
		}(t)
	}
	wg.Wait()

	sort.Slice(merged, func(i, j int) bool { return merged[i].Ts > merged[j].Ts })
	if len(merged) > limit {
		merged = merged[:limit]
	}
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
	}, nil)
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

// evalRecordsPeerInstances 返回某 datasource 上仍在心跳的全部引擎实例，
// 用于覆盖 hashring 重新分配之前的历史 owner（记录留在原节点磁盘上，不会跟着迁移）。
// 查询失败时返回 nil：兜底路径不该让主查询失败，当前 owner 已经单独纳入了。
func (rt *Router) evalRecordsPeerInstances(rule *models.AlertRule, dsId int64) []string {
	aliveSince := time.Now().Unix() - evalRecordsEngineAliveSec

	var instances []string
	var err error
	if rule.IsHostRule() || dsId == 0 {
		// host 规则的 worker 以 datasourceId=0 运行、按 engine_cluster 组成 hashring
		instances, err = models.AlertingEngineGetsInstances(rt.Ctx,
			"engine_cluster = ? and clock > ?", rt.Alert.Heartbeat.EngineName, aliveSince)
	} else {
		instances, err = models.AlertingEngineGetsInstances(rt.Ctx,
			"datasource_id = ? and clock > ?", dsId, aliveSince)
	}
	if err != nil {
		logger.Warningf("eval records: list alive engine instances for rule %d ds %d error: %v", rule.Id, dsId, err)
		return nil
	}
	return instances
}

// evalRecordsNodeErr 单个引擎节点查询失败信息。前端据此提示：
// 可登录该节点本机访问 /v1/n9e/eval-records 接口查看记录。
type evalRecordsNodeErr struct {
	Instance     string `json:"instance"`
	DatasourceId int64  `json:"datasource_id"`
	Error        string `json:"error"`
}

// forwardEvalRecords 向目标引擎节点转发查询，返回 (记录, 该节点是否启用 evallog, 错误)。
func (rt *Router) forwardEvalRecords(node string, ruleId, dsId, from, to, before int64, limit int) ([]evallog.EvalRecord, bool, error) {
	url := fmt.Sprintf("http://%s/v1/n9e/eval-records?rule_id=%d&datasource_id=%d&from=%d&to=%d&before=%d&limit=%d",
		node, ruleId, dsId, from, to, before, limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}

	for user, pass := range rt.HTTP.APIForService.BasicAuth {
		req.SetBasicAuth(user, pass)
		break
	}

	// 超时收窄：edge 节点可能被防火墙 drop，长超时会拖住整个请求
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("forward to %s failed: %v", node, err)
	}
	defer resp.Body.Close()

	// 多读 1 字节用于判断是否触到上限：LimitReader 截断时不报错，直接 Unmarshal
	// 会得到"JSON 语法错误"这种看不出根因的信息
	body, err := io.ReadAll(io.LimitReader(resp.Body, evalRecordsMaxRespBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > evalRecordsMaxRespBytes {
		return nil, false, fmt.Errorf("response from %s exceeds %d bytes, narrow the time range or lower limit",
			node, evalRecordsMaxRespBytes)
	}

	var result struct {
		Dat struct {
			List    []evallog.EvalRecord `json:"list"`
			Enabled *bool                `json:"enabled"`
		} `json:"dat"`
		Err string `json:"err"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, false, err
	}
	if result.Err != "" {
		return nil, false, fmt.Errorf("%s", result.Err)
	}
	// 老版本引擎不返回 enabled 字段，缺省按启用处理，保持向后兼容
	enabled := result.Dat.Enabled == nil || *result.Dat.Enabled
	return result.Dat.List, enabled, nil
}
