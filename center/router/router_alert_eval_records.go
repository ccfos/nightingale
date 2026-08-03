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

	// 解析各 datasource 对应的引擎节点并去重
	type dsNode struct {
		dsId int64
		node string
	}
	var targets []dsNode
	seen := make(map[string]struct{})
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
		key := fmt.Sprintf("%d@%s", dsId, node)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, dsNode{dsId: dsId, node: node})
	}

	// 并发查询各目标节点（edge 节点不可达时靠短超时快速失败），
	// 失败节点不静默丢弃，透出给前端以区分"没有记录"和"取不到记录"
	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		merged    []evallog.EvalRecord
		instances []string
		nodeErrs  []evalRecordsNodeErr
	)
	for _, t := range targets {
		wg.Add(1)
		go func(t dsNode) {
			defer wg.Done()
			var recs []evallog.EvalRecord
			var err error
			if t.node == instance {
				recs, err = evallog.QueryRecords(ruleId, t.dsId, from*1000, to*1000, before, limit)
				if err == evallog.ErrNotEnabled {
					err = nil
				}
			} else {
				recs, err = rt.forwardEvalRecords(t.node, ruleId, t.dsId, from, to, before, limit)
			}

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				nodeErrs = append(nodeErrs, evalRecordsNodeErr{
					Instance:     t.node,
					DatasourceId: t.dsId,
					Error:        err.Error(),
				})
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

	ginx.NewRender(c).Data(gin.H{
		"list":      merged,
		"instances": instances,
		"errors":    nodeErrs,
	}, nil)
}

// evalRecordsNodeErr 单个引擎节点查询失败信息。前端据此提示：
// 可登录该节点本机访问 /v1/n9e/eval-records 接口查看记录。
type evalRecordsNodeErr struct {
	Instance     string `json:"instance"`
	DatasourceId int64  `json:"datasource_id"`
	Error        string `json:"error"`
}

func (rt *Router) forwardEvalRecords(node string, ruleId, dsId, from, to, before int64, limit int) ([]evallog.EvalRecord, error) {
	url := fmt.Sprintf("http://%s/v1/n9e/eval-records?rule_id=%d&datasource_id=%d&from=%d&to=%d&before=%d&limit=%d",
		node, ruleId, dsId, from, to, before, limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	for user, pass := range rt.HTTP.APIForService.BasicAuth {
		req.SetBasicAuth(user, pass)
		break
	}

	// 超时收窄：edge 节点可能被防火墙 drop，长超时会拖住整个请求
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forward to %s failed: %v", node, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024)) // 64MB limit
	if err != nil {
		return nil, err
	}

	var result struct {
		Dat struct {
			List []evallog.EvalRecord `json:"list"`
		} `json:"dat"`
		Err string `json:"err"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Err != "" {
		return nil, fmt.Errorf("%s", result.Err)
	}
	return result.Dat.List, nil
}
