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
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

const boardTokenBidKey = "board_share_bid"

// BoardTokenDetect 解析并校验 __token，有效则把绑定的 board id 放进 context；
// 本身不拦截，后续的 SkipIfBoardToken(auth/user) 据此决定是否跳过登录鉴权。
// 注意不能在单个中间件里"校验失败就内联调用 rt.auth()(c)"——auth 成功时内部
// 会 c.Next()，把最终 handler 提前执行掉，user 对象反而在其之后才注入。
//
// 导出为包级函数供 n9e-plus 复用：plus 的仪表盘查询接口在 /api/n9e-plus 下，
// 需要同一套分享 token 判定（plus 版前端的 N9E_PATHNAME 即 n9e-plus）。
func BoardTokenDetect(bctx *ctx.Context) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := ginx.QueryStr(c, "__token", ""); token != "" {
			st, err := models.GetSourceTokenByToken(bctx, models.SourceTypeBoard, token)
			if err == nil && st != nil && !st.IsExpired() {
				if bid, e := strconv.ParseInt(st.SourceId, 10, 64); e == nil && bid > 0 {
					c.Set(boardTokenBidKey, bid)
				}
			}
		}
		c.Next()
	}
}

// boardTokenVerdict 一条分享令牌对目标仪表盘的效力判定
type boardTokenVerdict int

const (
	// boardTokenPass 令牌不存在/已注销/已过期：按「没带令牌」处理，继续走原有的
	// public/登录判定，行为与不带令牌完全一致（过期链接仍跳登录页，登录用户带失效
	// 链接也能正常访问）
	boardTokenPass boardTokenVerdict = iota
	// boardTokenAllow 令牌绑定的正是目标板：匿名放行
	boardTokenAllow
	// boardTokenDeny 令牌有效、但绑的是别的板：横向越权，就地拒绝
	boardTokenDeny
)

// judgeBoardToken 判定分享令牌 st 对 boardId 的效力。
//
// 关键在于 Deny 与 Pass 必须分开：令牌有效但绑别的板时若也当 Pass 处理、fall-through
// 到 public/登录判定，配了「匿名访问」(PublicCate=PublicAnonymous) 的板就会被任意一条
// 有效分享链接顺带打开，令牌作用域从「一个板」放大成「全部匿名板」。
//
// 抽成纯函数是为了让这条安全边界可单测——boardGet 本身要打库，覆盖不到。
func judgeBoardToken(st *models.SourceToken, boardId int64) boardTokenVerdict {
	if st == nil || st.IsExpired() {
		return boardTokenPass
	}

	// source_id 在签发时已规范化为十进制 board id（见 sourceTokenAdd），这里按数值
	// 比对而非字符串，避免存量形态差异造成误判；解析不出来一律按不匹配处理
	sid, err := strconv.ParseInt(st.SourceId, 10, 64)
	if err != nil || sid != boardId {
		return boardTokenDeny
	}

	return boardTokenAllow
}

func (rt *Router) boardTokenDetect() gin.HandlerFunc {
	return BoardTokenDetect(rt.Ctx)
}

// SkipIfBoardToken 请求已被分享 token 放行时跳过 mw（登录鉴权），否则照常执行
func SkipIfBoardToken(mw gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := boardTokenBid(c); ok {
			c.Next()
			return
		}
		mw(c)
	}
}

func skipIfBoardToken(mw gin.HandlerFunc) gin.HandlerFunc { return SkipIfBoardToken(mw) }

func boardTokenBid(c *gin.Context) (int64, bool) { return BoardTokenBid(c) }

// BoardTokenBid 返回当前请求经分享 token 放行时绑定的 board id
func BoardTokenBid(c *gin.Context) (int64, bool) {
	v, ok := c.Get(boardTokenBidKey)
	if !ok {
		return 0, false
	}
	bid, ok := v.(int64)
	return bid, ok
}

// CheckBoardTokenDsPerm 分享 token 请求的目标数据源必须属于板内集合；
// 非 token 请求（登录态或全局匿名）不做处理，由调用方原有逻辑兜底
func CheckBoardTokenDsPerm(bctx *ctx.Context, dsCache *memsto.DatasourceCacheType, c *gin.Context, dsId int64) {
	bid, ok := BoardTokenBid(c)
	if !ok {
		return
	}

	set, err := boardDsSet(bctx, dsCache, bid)
	ginx.Dangerous(err)

	if _, has := set[dsId]; !has {
		ginx.Bomb(http.StatusForbidden, "datasource is not referenced by the shared dashboard")
	}
}

func (rt *Router) checkBoardTokenDsPerm(c *gin.Context, dsId int64) {
	CheckBoardTokenDsPerm(rt.Ctx, rt.DatasourceCache, c, dsId)
}

// boardTokenQueryContext 为分享 token 请求准备查询上下文：校验数据源属于板内集合，
// 并置位 EnforceReadOnly，让 SQL 数据源在执行前走 sqlbase.ValidateReadOnly 严格
// 只读校验（各方言原有的关键字黑名单按空格切词，`DELETE\tFROM` 即可绕过，
// 不构成安全边界）。
//
// 无条件置位而非按 cate 判断，是刻意的 fail-closed：该标记由 dskit 的 SQL 执行
// 收口点（sqlbase.Query / clickhouse.Query / doris.Query）读取，凡是走这些收口点
// 的查询——包括将来新增的 SQL 类插件——都自动受保护；非 SQL 插件不读该标记，
// 因而没有副作用。若按 cate 清单置位，新增插件忘记登记就会静默失去保护。
//
// 返回是否为 token 请求，供调用方决定是否跳过基于登录用户的权限校验。
// 导出为包级函数供 n9e-plus 的仪表盘查询接口复用。
func BoardTokenQueryContext(bctx *ctx.Context, dsCache *memsto.DatasourceCacheType, c *gin.Context, dsIds ...int64) bool {
	if _, ok := BoardTokenBid(c); !ok {
		return false
	}

	for _, dsId := range dsIds {
		CheckBoardTokenDsPerm(bctx, dsCache, c, dsId)
	}

	cc, _ := dskittypes.CallContextFromCtx(c.Request.Context())
	cc.EnforceReadOnly = true
	c.Request = c.Request.WithContext(dskittypes.WithCallContext(c.Request.Context(), cc))

	return true
}

func (rt *Router) boardTokenQueryContext(c *gin.Context, dsIds ...int64) bool {
	return BoardTokenQueryContext(rt.Ctx, rt.DatasourceCache, c, dsIds...)
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

func boardDsSet(bctx *ctx.Context, dsCache *memsto.DatasourceCacheType, bid int64) (map[int64]struct{}, error) {
	boardDsSetMu.Lock()
	if item, ok := boardDsSetItems[bid]; ok && time.Now().Before(item.expireAt) {
		boardDsSetMu.Unlock()
		return item.set, nil
	}
	boardDsSetMu.Unlock()

	payload, err := models.BoardPayloadGet(bctx, bid)
	if err != nil {
		return nil, err
	}

	set := collectBoardDatasourceIds(payload, datasourceIdsByCate(dsCache))

	boardDsSetMu.Lock()
	boardDsSetItems[bid] = boardDsSetItem{set: set, expireAt: time.Now().Add(boardDsSetTTL)}
	boardDsSetMu.Unlock()

	return set, nil
}

func (rt *Router) boardDsSet(bid int64) (map[int64]struct{}, error) {
	return boardDsSet(rt.Ctx, rt.DatasourceCache, bid)
}

func datasourceIdsByCate(dsCache *memsto.DatasourceCacheType) func(string) []int64 {
	return func(cate string) []int64 {
		dss := dsCache.GetByCate(cate)
		ids := make([]int64, 0, len(dss))
		for _, ds := range dss {
			ids = append(ids, ds.Id)
		}
		return ids
	}
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

// IsReadOnlyProxyPath 判定分享 token 态下允许经 proxy 透传的只读查询路径。
// urlPath 是 /api/n9e/proxy/:id 之后的部分（含前导斜杠），如 /api/v1/query、
// /myindex/_search。只放行仪表盘渲染与变量取值实际需要的只读端点，写/管理类
// 端点（_bulk、_doc、/-/reload、/api/v1/admin/* 等）不在白名单即拒。
//
// 路径穿越必须在这里挡住，不能只靠调用方：gin 不会 clean URL.Path（路由命中时
// 不触发 RedirectFixedPath），%2e%2e 到 c.Param 时已被 net/url 解码成 ..，而
// dsProxy 的 director 又把这段路径原样拼给上游、http.Transport 按 RequestURI()
// 逐字发出——全链路无人归一化。于是 /api/v1/query/../../../-/reload 会以
// /api/v1/query 前缀骗过下面的匹配，上游若前置 nginx（会先归一化再路由）就真的
// 落到 /-/reload。判定放在函数内部，n9e-plus 的 dsProxy 调的是同一个导出函数，
// 无需两仓各改一遍。
func IsReadOnlyProxyPath(method, urlPath string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPost:
	default:
		return false
	}

	// 含 .. 段一律拒绝：无法保证「这里匹配到的路径」与「上游最终解析到的路由」
	// 是同一个，而这正是白名单唯一的安全价值
	if hasDotDotSegment(urlPath) {
		return false
	}

	// 归一化后再匹配，避免 /./ 、// 之类形态让前缀/后缀判定与实际转发对不上
	urlPath = path.Clean(urlPath)
	if !strings.HasPrefix(urlPath, "/") {
		return false
	}

	// 集群根路径只返回版本等元信息，没有副作用。ES 面板必须靠它拿版本号才能决定
	// date_histogram 用 interval 还是 fixed_interval（见 fe 的
	// dashboard/Renderer/datasource/elasticsearch）——挡掉这条，ES 8 上会退化成
	// 已被移除的 interval，整块面板画不出来
	if urlPath == "/" {
		return strings.ToUpper(method) == http.MethodGet
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

// hasDotDotSegment 判断路径里是否存在独立的 ".." 段。按段比较而不是 strings.Contains，
// 以免误伤 /myindex..old/_search 这类合法名字里含两个点的路径
func hasDotDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
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
