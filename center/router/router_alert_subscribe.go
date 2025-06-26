package router

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/strx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
)

// Return all, front-end search and paging
func (rt *Router) alertSubscribeGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	lst, err := models.AlertSubscribeGets(rt.Ctx, bgid)
	ginx.Dangerous(err)

	ugcache := make(map[int64]*models.UserGroup)
	rulecache := make(map[int64]string)

	for i := 0; i < len(lst); i++ {
		ginx.Dangerous(lst[i].FillUserGroups(rt.Ctx, ugcache))
		ginx.Dangerous(lst[i].FillRuleNames(rt.Ctx, rulecache))
		ginx.Dangerous(lst[i].FillDatasourceIds(rt.Ctx))
		ginx.Dangerous(lst[i].DB2FE())
	}

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertSubscribeGetsByGids(c *gin.Context) {
	gids := strx.IdsInt64ForAPI(ginx.QueryStr(c, "gids", ""), ",")
	if len(gids) > 0 {
		for _, gid := range gids {
			rt.bgroCheck(c, gid)
		}
	} else {
		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			var err error
			gids, err = models.MyBusiGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)

			if len(gids) == 0 {
				ginx.NewRender(c).Data([]int{}, nil)
				return
			}
		}
	}

	lst, err := models.AlertSubscribeGetsByBGIds(rt.Ctx, gids)
	ginx.Dangerous(err)

	ugcache := make(map[int64]*models.UserGroup)
	rulecache := make(map[int64]string)

	for i := 0; i < len(lst); i++ {
		ginx.Dangerous(lst[i].FillUserGroups(rt.Ctx, ugcache))
		ginx.Dangerous(lst[i].FillRuleNames(rt.Ctx, rulecache))
		ginx.Dangerous(lst[i].FillDatasourceIds(rt.Ctx))
		ginx.Dangerous(lst[i].DB2FE())
	}

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertSubscribeGet(c *gin.Context) {
	subid := ginx.UrlParamInt64(c, "sid")

	sub, err := models.AlertSubscribeGet(rt.Ctx, "id=?", subid)
	ginx.Dangerous(err)

	if sub == nil {
		ginx.NewRender(c, 404).Message("No such alert subscribe")
		return
	}

	ugcache := make(map[int64]*models.UserGroup)
	ginx.Dangerous(sub.FillUserGroups(rt.Ctx, ugcache))

	rulecache := make(map[int64]string)
	ginx.Dangerous(sub.FillRuleNames(rt.Ctx, rulecache))
	ginx.Dangerous(sub.FillDatasourceIds(rt.Ctx))
	ginx.Dangerous(sub.DB2FE())

	ginx.NewRender(c).Data(sub, nil)
}

func (rt *Router) alertSubscribeAdd(c *gin.Context) {
	var f models.AlertSubscribe
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	f.CreateBy = username
	f.UpdateBy = username
	f.GroupId = ginx.UrlParamInt64(c, "id")

	if f.GroupId <= 0 {
		ginx.Bomb(http.StatusBadRequest, "group_id invalid")
	}

	ginx.NewRender(c).Message(f.Add(rt.Ctx))
}

type SubscribeTryRunForm struct {
	EventId         int64                 `json:"event_id" binding:"required"`
	SubscribeConfig models.AlertSubscribe `json:"config" binding:"required"`
}

func (rt *Router) alertSubscribeTryRun(c *gin.Context) {
	var f SubscribeTryRunForm
	ginx.BindJSON(c, &f)
	ginx.Dangerous(f.SubscribeConfig.Verify())

	hisEvent, err := models.AlertHisEventGetById(rt.Ctx, f.EventId)
	ginx.Dangerous(err)

	if hisEvent == nil {
		ginx.Bomb(http.StatusNotFound, "event not found")
	}

	curEvent := *hisEvent.ToCur()
	curEvent.SetTagsMap()

	lang := c.GetHeader("X-Language")

	// 先判断匹配条件
	if !f.SubscribeConfig.MatchCluster(curEvent.DatasourceId) {
		ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "event datasource not match"))
	}

	if len(f.SubscribeConfig.RuleIds) != 0 {
		match := false
		for _, rid := range f.SubscribeConfig.RuleIds {
			if rid == curEvent.RuleId {
				match = true
				break
			}
		}
		if !match {
			ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "event rule id not match"))
		}
	}

	// 匹配 tag
	f.SubscribeConfig.Parse()
	if !common.MatchTags(curEvent.TagsMap, f.SubscribeConfig.ITags) {
		ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "event tags not match"))
	}

	// 匹配group name
	if !common.MatchGroupsName(curEvent.GroupName, f.SubscribeConfig.IBusiGroups) {
		ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "event group name not match"))
	}

	// 检查严重级别（Severity）匹配
	if len(f.SubscribeConfig.SeveritiesJson) != 0 {
		match := false
		for _, s := range f.SubscribeConfig.SeveritiesJson {
			if s == curEvent.Severity || s == 0 {
				match = true
				break
			}
		}
		if !match {
			ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "event severity not match"))
		}
	}

	// 新版本通知规则
	if f.SubscribeConfig.NotifyVersion == 1 {
		if len(f.SubscribeConfig.NotifyRuleIds) == 0 {
			ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "no notify rules selected"))
		}

		for _, id := range f.SubscribeConfig.NotifyRuleIds {
			notifyRule, err := models.GetNotifyRule(rt.Ctx, id)
			if err != nil {
				ginx.Bomb(http.StatusNotFound, i18n.Sprintf(lang, "subscribe notify rule not found: %v", err))
			}

			for _, notifyConfig := range notifyRule.NotifyConfigs {
				_, err = SendNotifyChannelMessage(rt.Ctx, rt.UserCache, rt.UserGroupCache, notifyConfig, []*models.AlertCurEvent{&curEvent})
				if err != nil {
					ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "notify rule send error: %v", err))
				}
			}
		}

		ginx.NewRender(c).Data(i18n.Sprintf(lang, "event match subscribe and notification test ok"), nil)
		return
	}

	// 旧版通知方式
	f.SubscribeConfig.ModifyEvent(&curEvent)
	if len(curEvent.NotifyChannelsJSON) == 0 {
		ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "no notify channels selected"))
	}

	if len(curEvent.NotifyGroupsJSON) == 0 {
		ginx.Bomb(http.StatusOK, i18n.Sprintf(lang, "no notify groups selected"))
	}

	ancs := make([]string, 0, len(curEvent.NotifyChannelsJSON))
	ugids := strings.Fields(f.SubscribeConfig.UserGroupIds)
	ngids := make([]int64, 0)
	for i := 0; i < len(ugids); i++ {
		if gid, err := strconv.ParseInt(ugids[i], 10, 64); err == nil {
			ngids = append(ngids, gid)
		}
	}

	userGroups := rt.UserGroupCache.GetByUserGroupIds(ngids)
	uids := make([]int64, 0)
	for i := range userGroups {
		uids = append(uids, userGroups[i].UserIds...)
	}
	users := rt.UserCache.GetByUserIds(uids)
	for _, NotifyChannels := range curEvent.NotifyChannelsJSON {
		flag := true
		// ignore non-default channels
		switch NotifyChannels {
		case models.Dingtalk, models.Wecom, models.Feishu, models.Mm,
			models.Telegram, models.Email, models.FeishuCard:
			// do nothing
		default:
			continue
		}
		// default channels
		for ui := range users {
			if _, b := users[ui].ExtractToken(NotifyChannels); b {
				flag = false
				break
			}
		}
		if flag {
			ancs = append(ancs, NotifyChannels)
		}
	}
	if len(ancs) > 0 {
		ginx.Bomb(http.StatusBadRequest, i18n.Sprintf(lang, "all users missing notify channel configurations: %v", ancs))
	}

	ginx.NewRender(c).Data(i18n.Sprintf(lang, "event match subscribe and notify settings ok"), nil)
}

func (rt *Router) alertSubscribePut(c *gin.Context) {
	var fs []models.AlertSubscribe
	ginx.BindJSON(c, &fs)

	timestamp := time.Now().Unix()
	username := c.MustGet("username").(string)
	for i := 0; i < len(fs); i++ {
		fs[i].UpdateBy = username
		fs[i].UpdateAt = timestamp
		//After adding the function of batch subscription alert rules, rule_ids is used instead of rule_id.
		//When the subscription rules are updated, set rule_id=0 to prevent the wrong subscription caused by the old rule_id.
		fs[i].RuleId = 0
		ginx.Dangerous(fs[i].Update(
			rt.Ctx,
			"name",
			"disabled",
			"prod",
			"cate",
			"datasource_ids",
			"cluster",
			"rule_id",
			"rule_ids",
			"tags",
			"redefine_severity",
			"new_severity",
			"redefine_channels",
			"new_channels",
			"user_group_ids",
			"update_at",
			"update_by",
			"webhooks",
			"for_duration",
			"redefine_webhooks",
			"severities",
			"extra_config",
			"busi_groups",
			"note",
			"notify_rule_ids",
		))
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) alertSubscribeDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertSubscribeDel(rt.Ctx, f.Ids))
}

func (rt *Router) alertSubscribeGetsByService(c *gin.Context) {
	lst, err := models.AlertSubscribeGetsByService(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}
