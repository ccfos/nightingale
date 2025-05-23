package router

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/strx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func getUserGroupIds(ctx *gin.Context, rt *Router, myGroups bool) ([]int64, error) {
	if !myGroups {
		return nil, nil
	}
	me := ctx.MustGet("user").(*models.User)
	return models.MyGroupIds(rt.Ctx, me.Id)
}

func (rt *Router) alertCurEventsCard(c *gin.Context) {
	stime, etime := getTimeRange(c)
	severity := strx.IdsInt64ForAPI(ginx.QueryStr(c, "severity", ""), ",")
	query := ginx.QueryStr(c, "query", "")
	myGroups := ginx.QueryBool(c, "my_groups", false) // 是否只看自己组，默认false

	var gids []int64
	var err error
	if myGroups {
		gids, err = getUserGroupIds(c, rt, myGroups)
		ginx.Dangerous(err)
		if len(gids) == 0 {
			gids = append(gids, -1)
		}
	}

	viewId := ginx.QueryInt64(c, "view_id")

	alertView, err := models.GetAlertAggrViewByViewID(rt.Ctx, viewId)
	ginx.Dangerous(err)

	if alertView == nil {
		ginx.Bomb(http.StatusNotFound, "alert aggr view not found")
	}

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

	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView)
	ginx.Dangerous(err)

	// 最多获取50000个，获取太多也没啥意义
	list, err := models.AlertCurEventsGet(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, 0, query, 50000, 0, gids, []int64{})
	ginx.Dangerous(err)

	cardmap := make(map[string]*AlertCard)
	for _, event := range list {
		title, err := event.GenCardTitle(alertView.Rule)
		ginx.Dangerous(err)
		if _, has := cardmap[title]; has {
			cardmap[title].Total++
			cardmap[title].EventIds = append(cardmap[title].EventIds, event.Id)
			if event.Severity < cardmap[title].Severity {
				cardmap[title].Severity = event.Severity
			}
		} else {
			cardmap[title] = &AlertCard{
				Total:    1,
				EventIds: []int64{event.Id},
				Title:    title,
				Severity: event.Severity,
			}
		}
	}

	titles := make([]string, 0, len(cardmap))
	for title := range cardmap {
		titles = append(titles, title)
	}

	sort.Strings(titles)

	cards := make([]*AlertCard, len(titles))
	for i := 0; i < len(titles); i++ {
		cards[i] = cardmap[titles[i]]
	}

	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].Severity != cards[j].Severity {
			return cards[i].Severity < cards[j].Severity
		}
		return cards[i].Total > cards[j].Total
	})

	ginx.NewRender(c).Data(cards, nil)
}

type AlertCard struct {
	Title    string  `json:"title"`
	Total    int     `json:"total"`
	EventIds []int64 `json:"event_ids"`
	Severity int     `json:"severity"`
}

func (rt *Router) alertCurEventsCardDetails(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	list, err := models.AlertCurEventGetByIds(rt.Ctx, f.Ids)
	if err == nil {
		cache := make(map[int64]*models.UserGroup)
		for i := 0; i < len(list); i++ {
			list[i].FillNotifyGroups(rt.Ctx, cache)
		}
	}

	ginx.NewRender(c).Data(list, err)
}

// alertCurEventsGetByRid
func (rt *Router) alertCurEventsGetByRid(c *gin.Context) {
	rid := ginx.QueryInt64(c, "rid")
	dsId := ginx.QueryInt64(c, "dsid")
	ginx.NewRender(c).Data(models.AlertCurEventGetByRuleIdAndDsId(rt.Ctx, rid, dsId))
}

// 列表方式，拉取活跃告警
func (rt *Router) alertCurEventsList(c *gin.Context) {
	stime, etime := getTimeRange(c)
	severity := strx.IdsInt64ForAPI(ginx.QueryStr(c, "severity", ""), ",")
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	myGroups := ginx.QueryBool(c, "my_groups", false) // 是否只看自己组，默认false

	dsIds := queryDatasourceIds(c)

	eventIds := strx.IdsInt64ForAPI(ginx.QueryStr(c, "event_ids", ""), ",")

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

	var gids []int64
	var err error
	if myGroups {
		gids, err = getUserGroupIds(c, rt, myGroups)
		ginx.Dangerous(err)
		if len(gids) == 0 {
			gids = append(gids, -1)
		}
	}

	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView)
	ginx.Dangerous(err)

	total, err := models.AlertCurEventTotal(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, ruleId, query, gids, eventIds)
	ginx.Dangerous(err)

	list, err := models.AlertCurEventsGet(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, ruleId, query, limit, ginx.Offset(c, limit), gids, eventIds)
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

func (rt *Router) alertCurEventDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	rt.checkCurEventBusiGroupRWPermission(c, f.Ids)

	ginx.NewRender(c).Message(models.AlertCurEventDel(rt.Ctx, f.Ids))
}

func (rt *Router) checkCurEventBusiGroupRWPermission(c *gin.Context, ids []int64) {
	set := make(map[int64]struct{})

	// event group id is 0, ignore perm check
	set[0] = struct{}{}

	for i := 0; i < len(ids); i++ {
		event, err := models.AlertCurEventGetById(rt.Ctx, ids[i])
		ginx.Dangerous(err)
		if event == nil {
			continue
		}
		if _, has := set[event.GroupId]; !has {
			rt.bgrwCheck(c, event.GroupId)
			set[event.GroupId] = struct{}{}
		}
	}
}

func (rt *Router) alertCurEventGet(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	event, err := GetCurEventDetail(rt.Ctx, eid)
	ginx.NewRender(c).Data(event, err)
}

func GetCurEventDetail(ctx *ctx.Context, eid int64) (*models.AlertCurEvent, error) {
	event, err := models.AlertCurEventGetById(ctx, eid)
	if err != nil {
		return nil, err
	}

	if event == nil {
		return nil, fmt.Errorf("no such active event")
	}

	ruleConfig, needReset := models.FillRuleConfigTplName(ctx, event.RuleConfig)
	if needReset {
		event.RuleConfigJson = ruleConfig
	}

	event.LastEvalTime = event.TriggerTime
	event.NotifyVersion, err = GetEventNotifyVersion(ctx, event.RuleId, event.NotifyRuleIds)
	return event, err
}

func GetEventNotifyVersion(ctx *ctx.Context, ruleId int64, notifyRuleIds []int64) (int, error) {
	if len(notifyRuleIds) != 0 {
		// 如果存在 notify_rule_ids，则认为使用新的告警通知方式
		return 1, nil
	}

	rule, err := models.AlertRuleGetById(ctx, ruleId)
	if err != nil {
		return 0, err
	}
	return rule.NotifyVersion, nil
}

func (rt *Router) alertCurEventsStatistics(c *gin.Context) {

	ginx.NewRender(c).Data(models.AlertCurEventStatistics(rt.Ctx, time.Now()), nil)
}

func (rt *Router) alertCurEventDelByHash(c *gin.Context) {
	hash := ginx.QueryStr(c, "hash")
	ginx.NewRender(c).Message(models.AlertCurEventDelByHash(rt.Ctx, hash))
}
