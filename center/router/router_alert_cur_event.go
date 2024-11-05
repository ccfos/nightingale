package router

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func parseAggrRules(c *gin.Context) []*models.AggrRule {
	aggrRules := strings.Split(ginx.QueryStr(c, "rule", ""), "::") // e.g. field:group_name::field:severity::tagkey:ident

	if len(aggrRules) == 0 {
		ginx.Bomb(http.StatusBadRequest, "rule empty")
	}

	rules := make([]*models.AggrRule, len(aggrRules))
	for i := 0; i < len(aggrRules); i++ {
		pair := strings.Split(aggrRules[i], ":")
		if len(pair) != 2 {
			ginx.Bomb(http.StatusBadRequest, "rule invalid")
		}

		if !(pair[0] == "field" || pair[0] == "tagkey") {
			ginx.Bomb(http.StatusBadRequest, "rule invalid")
		}

		rules[i] = &models.AggrRule{
			Type:  pair[0],
			Value: pair[1],
		}
	}

	return rules
}

func (rt *Router) alertCurEventsCard(c *gin.Context) {
	stime, etime := getTimeRange(c)
	severity := ginx.QueryInt(c, "severity", -1)
	query := ginx.QueryStr(c, "query", "")
	dsIds := queryDatasourceIds(c)
	rules := parseAggrRules(c)

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
		cates, 0, query, 50000, 0)
	ginx.Dangerous(err)

	cardmap := make(map[string]*AlertCard)
	for _, event := range list {
		title := event.GenCardTitle(rules)
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
	severity := ginx.QueryInt(c, "severity", -1)
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

	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView)
	ginx.Dangerous(err)

	total, err := models.AlertCurEventTotal(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, ruleId, query)
	ginx.Dangerous(err)

	list, err := models.AlertCurEventsGet(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, ruleId, query, limit, ginx.Offset(c, limit))
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
	event, err := models.AlertCurEventGetById(rt.Ctx, eid)
	ginx.Dangerous(err)

	if event == nil {
		ginx.Bomb(404, "No such active event")
	}

	if !rt.Center.AnonymousAccess.AlertDetail && rt.Center.EventHistoryGroupView {
		rt.bgroCheck(c, event.GroupId)
	}

	ruleConfig, needReset := models.FillRuleConfigTplName(rt.Ctx, event.RuleConfig)
	if needReset {
		event.RuleConfigJson = ruleConfig
	}

	event.LastEvalTime = event.TriggerTime
	ginx.NewRender(c).Data(event, nil)
}

func (rt *Router) alertCurEventsStatistics(c *gin.Context) {

	ginx.NewRender(c).Data(models.AlertCurEventStatistics(rt.Ctx, time.Now()), nil)
}

func (rt *Router) alertCurEventDelByHash(c *gin.Context) {
	hash := ginx.QueryStr(c, "hash")
	ginx.NewRender(c).Message(models.AlertCurEventDelByHash(rt.Ctx, hash))
}
