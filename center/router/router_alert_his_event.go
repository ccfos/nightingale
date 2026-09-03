package router

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/exp/slices"
)

// 历史告警 count(*) 缓存：大表上每次翻页都重算 count 代价很高，
// 第一页实算并刷新缓存，翻页在 TTL 内直接复用
const hisTotalCacheTTL = 120 // seconds

type hisTotalCacheEntry struct {
	total    int64
	expireAt int64
}

var (
	hisTotalCacheMu sync.Mutex
	hisTotalCache   = make(map[string]hisTotalCacheEntry)
)

func hisTotalCacheGet(key string) (int64, bool) {
	hisTotalCacheMu.Lock()
	defer hisTotalCacheMu.Unlock()
	entry, has := hisTotalCache[key]
	if !has || entry.expireAt < time.Now().Unix() {
		return 0, false
	}
	return entry.total, true
}

func hisTotalCacheSet(key string, total int64) {
	now := time.Now().Unix()
	hisTotalCacheMu.Lock()
	defer hisTotalCacheMu.Unlock()
	if len(hisTotalCache) > 1000 {
		for k, v := range hisTotalCache {
			if v.expireAt < now {
				delete(hisTotalCache, k)
			}
		}
	}
	hisTotalCache[key] = hisTotalCacheEntry{total: total, expireAt: now + hisTotalCacheTTL}
}

func getTimeRange(c *gin.Context) (stime, etime int64) {
	stime = ginx.QueryInt64(c, "stime", 0)
	etime = ginx.QueryInt64(c, "etime", 0)
	hours := ginx.QueryInt64(c, "hours", 0)
	now := time.Now().Unix()
	if hours != 0 {
		stime = now - 3600*hours
		etime = now + 3600*24
	}

	if stime != 0 && etime == 0 {
		etime = now + 3600*24
	}
	return
}

func (rt *Router) alertHisEventsList(c *gin.Context) {
	stime, etime := getTimeRange(c)

	severity := ginx.QueryInt(c, "severity", -1)
	recovered := ginx.QueryInt(c, "is_recovered", -1)
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	dsIds := queryDatasourceIds(c)

	prod := ginx.QueryStr(c, "prods", "")
	if prod == "" {
		prod = ginx.QueryStr(c, "rule_prods", "")
	}

	prods := []string{}
	if prod != "" {
		prods = strings.Split(prod, ",")
	}

	cate := ginx.QueryStr(c, "cate", "$all")
	cates := []string{}
	if cate != "$all" {
		cates = strings.Split(cate, ",")
	}

	ruleId := ginx.QueryInt64(c, "rid", 0)

	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView, false)
	ginx.Dangerous(err)

	offset := ginx.Offset(c, limit)

	// hours 模式下 stime/etime 随请求时刻漂移，缓存 key 里按分钟取整，翻页请求才能命中
	cacheKey := fmt.Sprintf("%v|%v|%d|%d|%d|%d|%v|%v|%d|%s",
		prods, bgids, stime/60, etime/60, severity, recovered, dsIds, cates, ruleId, query)

	total, hit := int64(0), false
	if offset > 0 {
		total, hit = hisTotalCacheGet(cacheKey)
	}
	if !hit {
		total, err = models.AlertHisEventTotal(rt.Ctx, prods, bgids, stime, etime, severity,
			recovered, dsIds, cates, ruleId, query, []int64{})
		ginx.Dangerous(err)
		hisTotalCacheSet(cacheKey, total)
	}

	// 游标分页：传上一页最后一行的 last_eval_time 和 id，深翻页时不做 OFFSET 扫描
	cursorTime := ginx.QueryInt64(c, "cursor_time", 0)
	cursorId := ginx.QueryInt64(c, "cursor_id", 0)

	var list []models.AlertHisEvent
	if cursorTime > 0 && cursorId > 0 {
		list, err = models.AlertHisEventGetsByCursor(rt.Ctx, prods, bgids, stime, etime, severity,
			recovered, dsIds, cates, ruleId, query, cursorTime, cursorId, limit, []int64{})
	} else {
		list, err = models.AlertHisEventGets(rt.Ctx, prods, bgids, stime, etime, severity, recovered,
			dsIds, cates, ruleId, query, limit, offset, []int64{})
	}
	ginx.Dangerous(err)

	cache := make(map[int64]*models.UserGroup)
	for i := 0; i < len(list); i++ {
		list[i].FillNotifyGroups(rt.Ctx, cache)
	}

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type alertHisEventsDeleteForm struct {
	Severities []int `json:"severities"`
	Timestamp  int64 `json:"timestamp" binding:"required"`
}

func (rt *Router) alertHisEventsDelete(c *gin.Context) {
	var f alertHisEventsDeleteForm
	ginx.BindJSON(c, &f)
	// 校验
	if f.Timestamp == 0 {
		ginx.Bomb(http.StatusBadRequest, "timestamp parameter is required")
		return
	}

	user := c.MustGet("user").(*models.User)

	// timestamp 不允许超过当前时间，否则清理期间新触发的事件会绕过下面的活跃告警快照
	timestamp := f.Timestamp
	if now := time.Now().Unix(); timestamp > now {
		timestamp = now
	}

	// 启动后台清理任务
	go func() {
		// 快照活跃告警 id 集合（cur.id 即对应的 his.id），活跃告警的历史记录跳过不删
		ids, err := models.AlertCurEventIds(rt.Ctx)
		if err != nil {
			logger.Errorf("Failed to delete alert history events: query active event ids fail, operator=%s, error=%v",
				user.Username, err)
			return
		}
		activeIds := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			activeIds[id] = struct{}{}
		}

		limit := 100
		var minId int64
		for {
			fetched, deleted, maxId, err := models.AlertHisEventBatchDelete(rt.Ctx, timestamp, f.Severities, minId, limit, activeIds)
			if err != nil {
				logger.Errorf("Failed to delete alert history events: operator=%s, timestamp=%d, severities=%v, error=%v",
					user.Username, timestamp, f.Severities, err)
				break
			}
			logger.Debugf("Successfully deleted alert history events: operator=%s, timestamp=%d, severities=%v, deleted=%d, skipped_active=%d",
				user.Username, timestamp, f.Severities, deleted, int64(fetched)-deleted)
			if fetched < limit {
				break // 已经删完
			}
			minId = maxId

			time.Sleep(100 * time.Millisecond) // 防止锁表
		}
	}()
	ginx.NewRender(c).Data("Alert history events deletion started", nil)
}

var TransferEventToCur func(*ctx.Context, *models.AlertHisEvent) *models.AlertCurEvent

func init() {
	TransferEventToCur = transferEventToCur
}

func transferEventToCur(ctx *ctx.Context, event *models.AlertHisEvent) *models.AlertCurEvent {
	cur := event.ToCur()
	return cur
}

func (rt *Router) alertHisEventGet(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	event, err := models.AlertHisEventGetById(rt.Ctx, eid)
	ginx.Dangerous(err)
	if event == nil {
		ginx.Bomb(404, "No such alert event")
	}

	hasPermission := HasPermission(rt.Ctx, c, "event", fmt.Sprintf("%d", eid), rt.Center.AnonymousAccess.AlertDetail)
	if !hasPermission {
		rt.auth()(c)
		rt.user()(c)
		rt.bgroCheck(c, event.GroupId)
	}

	ruleConfig, needReset := models.FillRuleConfigTplName(rt.Ctx, event.RuleConfig)
	if needReset {
		event.RuleConfigJson = ruleConfig
	}

	event.NotifyVersion, err = GetEventNotifyVersion(rt.Ctx, event.RuleId, event.NotifyRuleIds)
	ginx.Dangerous(err)

	event.NotifyRules, err = GetEventNotifyRuleNames(rt.Ctx, event.NotifyRuleIds)
	ginx.NewRender(c).Data(TransferEventToCur(rt.Ctx, event), err)
}

func GetBusinessGroupIds(c *gin.Context, ctx *ctx.Context, onlySelfGroupView bool, myGroups bool) ([]int64, error) {
	bgid := ginx.QueryInt64(c, "bgid", 0)
	var bgids []int64

	if strings.HasPrefix(c.Request.URL.Path, "/v1") {
		// 如果请求路径以 /v1 开头，不查询用户信息
		if bgid > 0 {
			return []int64{bgid}, nil
		}

		return bgids, nil
	}

	user := c.MustGet("user").(*models.User)
	if myGroups || (onlySelfGroupView && !user.IsAdmin()) {
		// 1. 页面上勾选了我的业务组，需要查询用户所属的业务组
		// 2. 如果 onlySelfGroupView 为 true，表示只允许查询用户所属的业务组
		bussGroupIds, err := models.MyBusiGroupIds(ctx, user.Id)
		if err != nil {
			return nil, err
		}

		if len(bussGroupIds) == 0 {
			// 如果没查到用户属于任何业务组，需要返回一个0，否则会导致查询到全部告警历史
			return []int64{0}, nil
		}

		if bgid > 0 {
			if !slices.Contains(bussGroupIds, bgid) && !user.IsAdmin() {
				return nil, fmt.Errorf("business group ID not allowed")
			}

			return []int64{bgid}, nil
		}

		return bussGroupIds, nil
	}

	if bgid > 0 {
		return []int64{bgid}, nil
	}

	return bgids, nil
}
