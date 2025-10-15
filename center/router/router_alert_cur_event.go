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
	"github.com/toolkits/pkg/logger"
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

	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView, myGroups)
	ginx.Dangerous(err)

	// 最多获取50000个，获取太多也没啥意义
	list, err := models.AlertCurEventsGet(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, 0, query, 50000, 0, []int64{})
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

		if cardmap[title].Severity < 1 {
			cardmap[title].Severity = 3
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

	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView, myGroups)
	ginx.Dangerous(err)

	total, err := models.AlertCurEventTotal(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, ruleId, query, eventIds)
	ginx.Dangerous(err)

	list, err := models.AlertCurEventsGet(rt.Ctx, prods, bgids, stime, etime, severity, dsIds,
		cates, ruleId, query, limit, ginx.Offset(c, limit), eventIds)
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

	hasPermission := HasPermission(rt.Ctx, c, "event", fmt.Sprintf("%d", eid), rt.Center.AnonymousAccess.AlertDetail)
	if !hasPermission {
		rt.auth()(c)
		rt.user()(c)
		rt.bgroCheck(c, event.GroupId)
	}

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
	ginx.Dangerous(err)

	event.NotifyRules, err = GetEventNorifyRuleNames(ctx, event.NotifyRuleIds)
	return event, err
}

func GetEventNorifyRuleNames(ctx *ctx.Context, notifyRuleIds []int64) ([]*models.EventNotifyRule, error) {
	notifyRuleNames := make([]*models.EventNotifyRule, 0)
	notifyRules, err := models.NotifyRulesGet(ctx, "id in ?", notifyRuleIds)
	if err != nil {
		return nil, err
	}

	for _, notifyRule := range notifyRules {
		notifyRuleNames = append(notifyRuleNames, &models.EventNotifyRule{
			Id:   notifyRule.ID,
			Name: notifyRule.Name,
		})
	}
	return notifyRuleNames, nil
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

func (rt *Router) eventTagKeys(c *gin.Context) {
	// 获取最近1天的活跃告警事件
	now := time.Now().Unix()
	stime := now - 24*3600
	etime := now

	// 获取用户可见的业务组ID列表
	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView, false)
	if err != nil {
		logger.Warningf("failed to get business group ids: %v", err)
		ginx.NewRender(c).Data([]string{"ident", "app", "service", "instance"}, nil)
		return
	}

	// 查询活跃告警事件，限制数量以提高性能
	events, err := models.AlertCurEventsGet(rt.Ctx, []string{}, bgids, stime, etime, []int64{}, []int64{}, []string{}, 0, "", 200, 0, []int64{})
	if err != nil {
		logger.Warningf("failed to get current alert events: %v", err)
		ginx.NewRender(c).Data([]string{"ident", "app", "service", "instance"}, nil)
		return
	}

	// 如果没有查到事件，返回默认标签
	if len(events) == 0 {
		ginx.NewRender(c).Data([]string{"ident", "app", "service", "instance"}, nil)
		return
	}

	// 收集所有标签键并去重
	tagKeys := make(map[string]struct{})
	for _, event := range events {
		for key := range event.TagsMap {
			tagKeys[key] = struct{}{}
		}
	}

	// 转换为字符串切片
	var result []string
	for key := range tagKeys {
		result = append(result, key)
	}

	// 如果没有收集到任何标签键，返回默认值
	if len(result) == 0 {
		result = []string{"ident", "app", "service", "instance"}
	}

	ginx.NewRender(c).Data(result, nil)
}

func (rt *Router) eventTagValues(c *gin.Context) {
	// 获取标签key
	tagKey := ginx.QueryStr(c, "key")

	// 获取最近1天的活跃告警事件
	now := time.Now().Unix()
	stime := now - 24*3600
	etime := now

	// 获取用户可见的业务组ID列表
	bgids, err := GetBusinessGroupIds(c, rt.Ctx, rt.Center.EventHistoryGroupView, false)
	if err != nil {
		logger.Warningf("failed to get business group ids: %v", err)
		ginx.NewRender(c).Data([]string{}, nil)
		return
	}

	// 查询活跃告警事件，获取更多数据以保证统计准确性
	events, err := models.AlertCurEventsGet(rt.Ctx, []string{}, bgids, stime, etime, []int64{}, []int64{}, []string{}, 0, "", 1000, 0, []int64{})
	if err != nil {
		logger.Warningf("failed to get current alert events: %v", err)
		ginx.NewRender(c).Data([]string{}, nil)
		return
	}

	// 如果没有查到事件，返回空数组
	if len(events) == 0 {
		ginx.NewRender(c).Data([]string{}, nil)
		return
	}

	// 统计标签值出现次数
	valueCount := make(map[string]int)
	for _, event := range events {
		// TagsMap已经在AlertCurEventsGet中处理，直接使用
		if value, exists := event.TagsMap[tagKey]; exists && value != "" {
			valueCount[value]++
		}
	}

	// 转换为切片并按出现次数降序排序
	type tagValue struct {
		value string
		count int
	}

	tagValues := make([]tagValue, 0, len(valueCount))
	for value, count := range valueCount {
		tagValues = append(tagValues, tagValue{value, count})
	}

	// 按出现次数降序排序
	sort.Slice(tagValues, func(i, j int) bool {
		return tagValues[i].count > tagValues[j].count
	})

	// 只取Top20并转换为字符串数组
	limit := 20
	if len(tagValues) < limit {
		limit = len(tagValues)
	}

	result := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, tagValues[i].value)
	}

	ginx.NewRender(c).Data(result, nil)
}
