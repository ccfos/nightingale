package router

import (
	"fmt"
	"github.com/toolkits/pkg/logger"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"golang.org/x/exp/slices"
)

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

	total, err := models.AlertHisEventTotal(rt.Ctx, prods, bgids, stime, etime, severity,
		recovered, dsIds, cates, ruleId, query)
	ginx.Dangerous(err)

	list, err := models.AlertHisEventGets(rt.Ctx, prods, bgids, stime, etime, severity, recovered,
		dsIds, cates, ruleId, query, limit, ginx.Offset(c, limit))
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
	if !user.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}

	// 启动后台清理任务
	go func() {
		limit := 100
		for {
			n, err := models.AlertHisEventBatchDelete(rt.Ctx, f.Timestamp, f.Severities, limit)
			if err != nil {
				logger.Errorf("Failed to delete alert history events: %v", err)
				break
			}
			if n < int64(limit) {
				break // 已经删完
			}
			time.Sleep(100 * time.Millisecond) // 防止锁表
		}
	}()
	ginx.NewRender(c).Message("Alert history events deletion started")
}

func (rt *Router) alertHisEventGet(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	event, err := models.AlertHisEventGetById(rt.Ctx, eid)
	ginx.Dangerous(err)

	if event == nil {
		ginx.Bomb(404, "No such alert event")
	}

	if !rt.Center.AnonymousAccess.AlertDetail && rt.Center.EventHistoryGroupView {
		rt.bgroCheck(c, event.GroupId)
	}

	ruleConfig, needReset := models.FillRuleConfigTplName(rt.Ctx, event.RuleConfig)
	if needReset {
		event.RuleConfigJson = ruleConfig
	}

	event.NotifyVersion, err = GetEventNotifyVersion(rt.Ctx, event.RuleId, event.NotifyRuleIds)
	ginx.Dangerous(err)

	event.NotifyRules, err = GetEventNorifyRuleNames(rt.Ctx, event.NotifyRuleIds)
	ginx.NewRender(c).Data(event, err)
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
