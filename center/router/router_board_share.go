package router

// 仪表盘限时分享（source_token: source_type="board"）的数据查询通道。
//
// 携带有效 __token 的匿名请求可以走数据查询类接口，但目标数据源被收敛到
// 「该仪表盘引用的数据源集合」：面板里的字面量 datasourceValue，加上
// datasource / datasourceIdentifier 类型变量按 cate 展开的全部数据源。
// 变量的 regex 过滤器可能内嵌其他变量的插值，服务端不做精确复算，按 cate
// 放宽为超集——泄露面仍被限制在同类数据源内，且随 token 过期而失效。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

const boardTokenBidKey = "board_share_bid"

// boardTokenDetect 解析并校验 __token，有效则把绑定的 board id 放进 context；
// 本身不拦截，后续的 skipIfBoardToken(auth/user) 据此决定是否跳过登录鉴权。
// 注意不能在单个中间件里"校验失败就内联调用 rt.auth()(c)"——auth 成功时内部
// 会 c.Next()，把最终 handler 提前执行掉，user 对象反而在其之后才注入
func (rt *Router) boardTokenDetect() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := ginx.QueryStr(c, "__token", ""); token != "" {
			st, err := models.GetSourceTokenByToken(rt.Ctx, models.SourceTypeBoard, token)
			if err == nil && st != nil && !st.IsExpired() {
				if bid, e := strconv.ParseInt(st.SourceId, 10, 64); e == nil && bid > 0 {
					c.Set(boardTokenBidKey, bid)
				}
			}
		}
		c.Next()
	}
}

// skipIfBoardToken 请求已被分享 token 放行时跳过 mw（登录鉴权），否则照常执行
func skipIfBoardToken(mw gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := boardTokenBid(c); ok {
			c.Next()
			return
		}
		mw(c)
	}
}

// boardTokenBid 返回当前请求经分享 token 放行时绑定的 board id
func boardTokenBid(c *gin.Context) (int64, bool) {
	v, ok := c.Get(boardTokenBidKey)
	if !ok {
		return 0, false
	}
	bid, ok := v.(int64)
	return bid, ok
}

// checkBoardTokenDsPerm 分享 token 请求的目标数据源必须属于板内集合；
// 非 token 请求（登录态或全局匿名）不做处理，由调用方原有逻辑兜底
func (rt *Router) checkBoardTokenDsPerm(c *gin.Context, dsId int64) {
	bid, ok := boardTokenBid(c)
	if !ok {
		return
	}

	set, err := rt.boardDsSet(bid)
	ginx.Dangerous(err)

	if _, has := set[dsId]; !has {
		ginx.Bomb(http.StatusForbidden, "datasource is not referenced by the shared dashboard")
	}
}

// denyBoardTokenSQLCate 在 token 态拒绝 SQL 家族数据源查询。这些 cate 的查询
// 会把 SQL 原文经 sqlbase.Query 用数据源凭证下发，其唯一只读保护是按空格切词的
// BannedOp 黑名单（`DELETE\tFROM` 即可绕过），无法保证「匿名持链接者只读」，故
// 在匿名分享通道整体拒绝；非 token 请求（登录态）不受影响，仍走原有逻辑。
// 仅对走 sqlbase 弱黑名单的四个 cate 生效——tdengine/iotdb 不经该路径。
func denyBoardTokenSQLCate(c *gin.Context, cate string) {
	if _, ok := boardTokenBid(c); !ok {
		return
	}
	switch cate {
	case models.MYSQL, models.POSTGRESQL, models.CLICKHOUSE, models.DORIS:
		ginx.Bomb(http.StatusForbidden, "sql datasource is not allowed for anonymous dashboard sharing")
	}
}

// 板内数据源集合的进程内 TTL 缓存：一屏渲染会并发几十个 panel 请求，
// 共享一次 payload 解析；30s 内板配置的变更对分享链接可见性略有延迟，可接受
type boardDsSetItem struct {
	set      map[int64]struct{}
	expireAt time.Time
}

var (
	boardDsSetMu    sync.Mutex
	boardDsSetItems = make(map[int64]boardDsSetItem)
)

const boardDsSetTTL = 30 * time.Second

func (rt *Router) boardDsSet(bid int64) (map[int64]struct{}, error) {
	boardDsSetMu.Lock()
	if item, ok := boardDsSetItems[bid]; ok && time.Now().Before(item.expireAt) {
		boardDsSetMu.Unlock()
		return item.set, nil
	}
	boardDsSetMu.Unlock()

	payload, err := models.BoardPayloadGet(rt.Ctx, bid)
	if err != nil {
		return nil, err
	}

	set := collectBoardDatasourceIds(payload, rt.datasourceIdsByCate)

	boardDsSetMu.Lock()
	boardDsSetItems[bid] = boardDsSetItem{set: set, expireAt: time.Now().Add(boardDsSetTTL)}
	boardDsSetMu.Unlock()

	return set, nil
}

func (rt *Router) datasourceIdsByCate(cate string) []int64 {
	dss := rt.DatasourceCache.GetByCate(cate)
	ids := make([]int64, 0, len(dss))
	for _, ds := range dss {
		ids = append(ids, ds.Id)
	}
	return ids
}

// collectBoardDatasourceIds 从 board payload 提取引用的数据源 id 集合。
// expandCate 注入 cate -> 数据源 id 列表的查询（生产走 DatasourceCache）
func collectBoardDatasourceIds(payload string, expandCate func(string) []int64) map[int64]struct{} {
	set := make(map[int64]struct{})
	if strings.TrimSpace(payload) == "" {
		return set
	}

	// 字面量 datasourceValue：递归遍历整个 payload，不绑定具体 schema
	// （panel 可能嵌套在 row 下）；"${var}" 形式的引用交由下面的变量展开覆盖
	var root interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return set
	}
	collectDatasourceValues(root, set)

	var conf struct {
		Var []struct {
			Type       string `json:"type"`
			Definition string `json:"definition"`
			Datasource struct {
				Value interface{} `json:"value"`
			} `json:"datasource"`
		} `json:"var"`
	}
	if err := json.Unmarshal([]byte(payload), &conf); err == nil {
		for _, v := range conf.Var {
			// datasource / datasourceIdentifier 变量按 cate 展开成同类数据源集合
			if (v.Type == "datasource" || v.Type == "datasourceIdentifier") && v.Definition != "" {
				for _, id := range expandCate(v.Definition) {
					set[id] = struct{}{}
				}
			}
			// query 等变量把自己使用的数据源存为 var[].datasource.value（可为字面量 id）。
			// proxy 通道会按变量请求的数据源校验板内集合，这类只被变量引用、不被任何
			// 面板引用的数据源必须一并纳入，否则变量取值的 proxy 请求会被误判为板外。
			addLiteralDatasourceValue(v.Datasource.Value, set)
		}
	}

	return set
}

// addLiteralDatasourceValue 把字面量数据源 id（number 或数字字符串）并入集合；
// "${var}" 之类的变量引用不是字面量 id，跳过（其引用的 datasource 变量已单独展开）
func addLiteralDatasourceValue(v interface{}, set map[int64]struct{}) {
	switch t := v.(type) {
	case float64:
		if t > 0 {
			set[int64(t)] = struct{}{}
		}
	case string:
		if id, err := strconv.ParseInt(t, 10, 64); err == nil && id > 0 {
			set[id] = struct{}{}
		}
	}
}

// isReadOnlyProxyPath 判定分享 token 态下允许经 proxy 透传的只读查询路径。
// urlPath 是 /api/n9e/proxy/:id 之后的部分（含前导斜杠），如 /api/v1/query、
// /myindex/_search。只放行仪表盘渲染与变量取值实际需要的只读端点，写/管理类
// 端点（_bulk、_doc、/-/reload、/api/v1/admin/* 等）不在白名单即拒。
func isReadOnlyProxyPath(method, urlPath string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPost:
	default:
		return false
	}

	// Elasticsearch 只读查询端点（_search / _msearch，POST 但只读）
	if strings.HasSuffix(urlPath, "/_search") || strings.HasSuffix(urlPath, "/_msearch") {
		return true
	}

	// Prometheus / Loki / VictoriaMetrics 的只读 HTTP API
	readonlyPrefixes := []string{
		"/api/v1/query", // 覆盖 query 与 query_range
		"/api/v1/labels",
		"/api/v1/label/", // .../{name}/values
		"/api/v1/series",
		"/api/v1/metadata",
		"/api/v1/status/buildinfo",
	}
	for _, p := range readonlyPrefixes {
		if strings.HasPrefix(urlPath, p) {
			return true
		}
	}

	// VictoriaLogs 只读查询
	if strings.HasPrefix(urlPath, "/select/logsql/") {
		return true
	}

	return false
}

func collectDatasourceValues(node interface{}, out map[int64]struct{}) {
	switch v := node.(type) {
	case map[string]interface{}:
		for k, val := range v {
			if k == "datasourceValue" {
				if f, ok := val.(float64); ok && f > 0 {
					out[int64(f)] = struct{}{}
				}
			}
			collectDatasourceValues(val, out)
		}
	case []interface{}:
		for _, item := range v {
			collectDatasourceValues(item, out)
		}
	}
}
