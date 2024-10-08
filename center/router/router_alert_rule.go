package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/writer"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/str"
)

// Return all, front-end search and paging
func (rt *Router) alertRuleGets(c *gin.Context) {
	busiGroupId := ginx.UrlParamInt64(c, "id")
	ars, err := models.AlertRuleGets(rt.Ctx, busiGroupId)
	if err == nil {
		cache := make(map[int64]*models.UserGroup)
		for i := 0; i < len(ars); i++ {
			ars[i].FillNotifyGroups(rt.Ctx, cache)
			ars[i].FillSeverities()
		}
	}
	ginx.NewRender(c).Data(ars, err)
}

func getAlertCueEventTimeRange(c *gin.Context) (stime, etime int64) {
	stime = ginx.QueryInt64(c, "stime", 0)
	etime = ginx.QueryInt64(c, "etime", 0)
	if etime == 0 {
		etime = time.Now().Unix()
	}
	if stime == 0 || stime >= etime {
		stime = etime - 30*24*int64(time.Hour.Seconds())
	}
	return
}

func (rt *Router) alertRuleGetsByGids(c *gin.Context) {
	gids := str.IdsInt64(ginx.QueryStr(c, "gids", ""), ",")
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

	ars, err := models.AlertRuleGetsByBGIds(rt.Ctx, gids)
	if err == nil {
		cache := make(map[int64]*models.UserGroup)
		rids := make([]int64, 0, len(ars))
		names := make([]string, 0, len(ars))
		for i := 0; i < len(ars); i++ {
			ars[i].FillNotifyGroups(rt.Ctx, cache)
			ars[i].FillSeverities()
			rids = append(rids, ars[i].Id)
			names = append(names, ars[i].UpdateBy)
		}

		stime, etime := getAlertCueEventTimeRange(c)
		cnt := models.AlertCurEventCountByRuleId(rt.Ctx, rids, stime, etime)
		if cnt != nil {
			for i := 0; i < len(ars); i++ {
				ars[i].CurEventCount = cnt[ars[i].Id]
			}
		}

		users := models.UserMapGet(rt.Ctx, "username in (?)", names)
		if users != nil {
			for i := 0; i < len(ars); i++ {
				if user, exist := users[ars[i].UpdateBy]; exist {
					ars[i].UpdateByNickname = user.Nickname
				}
			}
		}
	}
	ginx.NewRender(c).Data(ars, err)
}

func (rt *Router) alertRulesGetByService(c *gin.Context) {
	prods := []string{}
	prodStr := ginx.QueryStr(c, "prods", "")
	if prodStr != "" {
		prods = strings.Split(ginx.QueryStr(c, "prods", ""), ",")
	}

	query := ginx.QueryStr(c, "query", "")
	algorithm := ginx.QueryStr(c, "algorithm", "")
	cluster := ginx.QueryStr(c, "cluster", "")
	cate := ginx.QueryStr(c, "cate", "$all")
	cates := []string{}
	if cate != "$all" {
		cates = strings.Split(cate, ",")
	}

	disabled := ginx.QueryInt(c, "disabled", -1)
	ars, err := models.AlertRulesGetsBy(rt.Ctx, prods, query, algorithm, cluster, cates, disabled)
	if err == nil {
		cache := make(map[int64]*models.UserGroup)
		for i := 0; i < len(ars); i++ {
			ars[i].FillNotifyGroups(rt.Ctx, cache)
		}
	}
	ginx.NewRender(c).Data(ars, err)
}

// single or import
func (rt *Router) alertRuleAddByFE(c *gin.Context) {
	username := c.MustGet("username").(string)

	var lst []models.AlertRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	bgid := ginx.UrlParamInt64(c, "id")
	reterr := rt.alertRuleAdd(lst, username, bgid, c.GetHeader("X-Language"))

	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) alertRuleAddByImport(c *gin.Context) {
	username := c.MustGet("username").(string)

	var lst []models.AlertRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	bgid := ginx.UrlParamInt64(c, "id")
	reterr := rt.alertRuleAdd(lst, username, bgid, c.GetHeader("X-Language"))

	ginx.NewRender(c).Data(reterr, nil)
}

type promRuleForm struct {
	Payload       string  `json:"payload" binding:"required"`
	DatasourceIds []int64 `json:"datasource_ids" binding:"required"`
	Disabled      int     `json:"disabled" binding:"gte=0,lte=1"`
}

func (rt *Router) alertRuleAddByImportPromRule(c *gin.Context) {
	var f promRuleForm
	ginx.Dangerous(c.BindJSON(&f))

	var pr struct {
		Groups []models.PromRuleGroup `yaml:"groups"`
	}
	err := yaml.Unmarshal([]byte(f.Payload), &pr)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "invalid yaml format, please use the example format. err: %v", err)
	}

	if len(pr.Groups) == 0 {
		ginx.Bomb(http.StatusBadRequest, "input yaml is empty")
	}

	lst := models.DealPromGroup(pr.Groups, f.DatasourceIds, f.Disabled)
	username := c.MustGet("username").(string)
	bgid := ginx.UrlParamInt64(c, "id")
	ginx.NewRender(c).Data(rt.alertRuleAdd(lst, username, bgid, c.GetHeader("X-Language")), nil)
}

func (rt *Router) alertRuleAddByService(c *gin.Context) {
	var lst []models.AlertRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}
	reterr := rt.alertRuleAddForService(lst, "")
	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) alertRuleAddOneByService(c *gin.Context) {
	var f models.AlertRule
	ginx.BindJSON(c, &f)

	err := f.FE2DB()
	ginx.Dangerous(err)

	err = f.Add(rt.Ctx)
	ginx.NewRender(c).Data(f.Id, err)
}

func (rt *Router) alertRuleAddForService(lst []models.AlertRule, username string) map[string]string {
	count := len(lst)
	// alert rule name -> error string
	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		lst[i].Id = 0
		if username != "" {
			lst[i].CreateBy = username
			lst[i].UpdateBy = username
		}

		if err := lst[i].FE2DB(); err != nil {
			reterr[lst[i].Name] = err.Error()
			continue
		}

		if err := lst[i].Add(rt.Ctx); err != nil {
			reterr[lst[i].Name] = err.Error()
		} else {
			reterr[lst[i].Name] = ""
		}
	}
	return reterr
}

func (rt *Router) alertRuleAdd(lst []models.AlertRule, username string, bgid int64, lang string) map[string]string {
	count := len(lst)
	// alert rule name -> error string
	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		lst[i].Id = 0
		lst[i].GroupId = bgid
		if username != "" {
			lst[i].CreateBy = username
			lst[i].UpdateBy = username
		}

		if err := lst[i].FE2DB(); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(lang, err.Error())
			continue
		}

		if err := lst[i].Add(rt.Ctx); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(lang, err.Error())
		} else {
			reterr[lst[i].Name] = ""
		}
	}
	return reterr
}

func (rt *Router) alertRuleDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	// param(busiGroupId) for protect
	ginx.NewRender(c).Message(models.AlertRuleDels(rt.Ctx, f.Ids, ginx.UrlParamInt64(c, "id")))
}

func (rt *Router) alertRuleDelByService(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()
	ginx.NewRender(c).Message(models.AlertRuleDels(rt.Ctx, f.Ids))
}

func (rt *Router) alertRulePutByFE(c *gin.Context) {
	var f models.AlertRule
	ginx.BindJSON(c, &f)

	arid := ginx.UrlParamInt64(c, "arid")
	ar, err := models.AlertRuleGetById(rt.Ctx, arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}

	rt.bgrwCheck(c, ar.GroupId)

	f.UpdateBy = c.MustGet("username").(string)
	ginx.NewRender(c).Message(ar.Update(rt.Ctx, f))
}

func (rt *Router) alertRulePutByService(c *gin.Context) {
	var f models.AlertRule
	ginx.BindJSON(c, &f)

	arid := ginx.UrlParamInt64(c, "arid")
	ar, err := models.AlertRuleGetById(rt.Ctx, arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}
	ginx.NewRender(c).Message(ar.Update(rt.Ctx, f))
}

type alertRuleFieldForm struct {
	Ids    []int64                `json:"ids"`
	Fields map[string]interface{} `json:"fields"`
	Action string                 `json:"action"`
}

// update one field: cluster note severity disabled prom_eval_interval prom_for_duration notify_channels notify_groups notify_recovered notify_repeat_step callbacks runbook_url append_tags
func (rt *Router) alertRulePutFields(c *gin.Context) {
	var f alertRuleFieldForm
	ginx.BindJSON(c, &f)

	if len(f.Fields) == 0 {
		ginx.Bomb(http.StatusBadRequest, "fields empty")
	}

	f.Fields["update_by"] = c.MustGet("username").(string)
	f.Fields["update_at"] = time.Now().Unix()

	for i := 0; i < len(f.Ids); i++ {
		ar, err := models.AlertRuleGetById(rt.Ctx, f.Ids[i])
		ginx.Dangerous(err)

		if ar == nil {
			continue
		}

		if f.Action == "update_triggers" {
			if triggers, has := f.Fields["triggers"]; has {
				originRule := ar.RuleConfigJson.(map[string]interface{})
				originRule["triggers"] = triggers
				b, err := json.Marshal(originRule)
				ginx.Dangerous(err)
				ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, map[string]interface{}{"rule_config": string(b)}))
				continue
			}
		}

		if f.Action == "annotations_add" {
			if annotations, has := f.Fields["annotations"]; has {
				annotationsMap := annotations.(map[string]interface{})
				for k, v := range annotationsMap {
					ar.AnnotationsJSON[k] = v.(string)
				}
				b, err := json.Marshal(ar.AnnotationsJSON)
				ginx.Dangerous(err)
				ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, map[string]interface{}{"annotations": string(b)}))
				continue
			}
		}

		if f.Action == "annotations_del" {
			if annotations, has := f.Fields["annotations"]; has {
				annotationsKeys := annotations.(map[string]interface{})
				for key := range annotationsKeys {
					delete(ar.AnnotationsJSON, key)
				}
				b, err := json.Marshal(ar.AnnotationsJSON)
				ginx.Dangerous(err)
				ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, map[string]interface{}{"annotations": string(b)}))
				continue
			}
		}

		if f.Action == "callback_add" {
			// 增加一个 callback 地址
			if callbacks, has := f.Fields["callbacks"]; has {
				callback := callbacks.(string)
				if !strings.Contains(ar.Callbacks, callback) {
					ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, map[string]interface{}{"callbacks": ar.Callbacks + " " + callback}))
					continue
				}
			}
		}

		if f.Action == "callback_del" {
			// 删除一个 callback 地址
			if callbacks, has := f.Fields["callbacks"]; has {
				callback := callbacks.(string)
				ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, map[string]interface{}{"callbacks": strings.ReplaceAll(ar.Callbacks, callback, "")}))
				continue
			}
		}

		for k, v := range f.Fields {
			ginx.Dangerous(ar.UpdateColumn(rt.Ctx, k, v))
		}
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) alertRuleGet(c *gin.Context) {
	arid := ginx.UrlParamInt64(c, "arid")

	ar, err := models.AlertRuleGetById(rt.Ctx, arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}

	err = ar.FillNotifyGroups(rt.Ctx, make(map[int64]*models.UserGroup))
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(ar, err)
}

func (rt *Router) alertRulePureGet(c *gin.Context) {
	arid := ginx.UrlParamInt64(c, "arid")

	ar, err := models.AlertRuleGetById(rt.Ctx, arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}

	ginx.NewRender(c).Data(ar, err)
}

// pre validation before save rule
func (rt *Router) alertRuleValidation(c *gin.Context) {
	var f models.AlertRule //new
	ginx.BindJSON(c, &f)

	if len(f.NotifyChannelsJSON) > 0 && len(f.NotifyGroupsJSON) > 0 { //Validation NotifyChannels
		ngids := make([]int64, 0, len(f.NotifyChannelsJSON))
		for i := range f.NotifyGroupsJSON {
			id, _ := strconv.ParseInt(f.NotifyGroupsJSON[i], 10, 64)
			ngids = append(ngids, id)
		}
		userGroups := rt.UserGroupCache.GetByUserGroupIds(ngids)
		uids := make([]int64, 0)
		for i := range userGroups {
			uids = append(uids, userGroups[i].UserIds...)
		}
		users := rt.UserCache.GetByUserIds(uids)
		//If any users have a certain notify channel's token, it will be okay. Otherwise, this notify channel is absent of tokens.
		ancs := make([]string, 0, len(f.NotifyChannelsJSON)) //absent Notify Channels
		for i := range f.NotifyChannelsJSON {
			flag := true
			//ignore non-default channels
			switch f.NotifyChannelsJSON[i] {
			case models.Dingtalk, models.Wecom, models.Feishu, models.Mm,
				models.Telegram, models.Email, models.FeishuCard:
				// do nothing
			default:
				continue
			}
			//default channels
			for ui := range users {
				if _, b := users[ui].ExtractToken(f.NotifyChannelsJSON[i]); b {
					flag = false
					break
				}
			}
			if flag {
				ancs = append(ancs, f.NotifyChannelsJSON[i])
			}
		}

		if len(ancs) > 0 {
			ginx.NewRender(c).Message("All users are missing notify channel configurations. Please check for missing tokens (each channel should be configured with at least one user). %s", ancs)
			return
		}

	}

	ginx.NewRender(c).Message("")
}

func (rt *Router) alertRuleCallbacks(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	bussGroupIds, err := models.MyBusiGroupIds(rt.Ctx, user.Id)
	ginx.Dangerous(err)

	ars, err := models.AlertRuleGetsByBGIds(rt.Ctx, bussGroupIds)
	ginx.Dangerous(err)

	var callbacks []string
	callbackFilter := make(map[string]struct{})
	for i := range ars {
		for _, callback := range ars[i].CallbacksJSON {
			if _, ok := callbackFilter[callback]; !ok {
				callbackFilter[callback] = struct{}{}
				callbacks = append(callbacks, callback)
			}
		}
	}

	ginx.NewRender(c).Data(callbacks, nil)
}

type alertRuleTestForm struct {
	Configs []*pconf.RelabelConfig `json:"configs"`
	Tags    []string               `json:"tags"`
}

func (rt *Router) relabelTest(c *gin.Context) {
	var f alertRuleTestForm
	ginx.BindJSON(c, &f)

	if len(f.Tags) == 0 || len(f.Configs) == 0 {
		ginx.Bomb(http.StatusBadRequest, "relabel config is empty")
	}

	labels := make([]prompb.Label, len(f.Tags))
	for i, tag := range f.Tags {
		label := strings.SplitN(tag, "=", 2)
		if len(label) != 2 {
			ginx.Bomb(http.StatusBadRequest, "tag:%s format error", tag)
		}

		labels[i] = prompb.Label{Name: label[0], Value: label[1]}
	}

	for i := 0; i < len(f.Configs); i++ {
		if f.Configs[i].Replacement == "" {
			f.Configs[i].Replacement = "$1"
		}

		if f.Configs[i].Separator == "" {
			f.Configs[i].Separator = ";"
		}

		if f.Configs[i].Regex == "" {
			f.Configs[i].Regex = "(.*)"
		}
	}

	relabels := writer.Process(labels, f.Configs...)

	var tags []string
	for _, label := range relabels {
		tags = append(tags, fmt.Sprintf("%s=%s", label.Name, label.Value))
	}

	ginx.NewRender(c).Data(tags, nil)
}

type identListForm struct {
	Ids       []int64  `json:"ids"`
	IdentList []string `json:"ident_list"`
}

func containsIdentOperator(s string) bool {
	pattern := `ident\s*(!=|!~|=~)`
	matched, err := regexp.MatchString(pattern, s)
	if err != nil {
		return false
	}
	return matched
}

func (rt *Router) cloneToMachine(c *gin.Context) {
	var f identListForm
	ginx.BindJSON(c, &f)

	if len(f.IdentList) == 0 {
		ginx.Bomb(http.StatusBadRequest, "ident_list is empty")
	}

	alertRules, err := models.AlertRuleGetsByIds(rt.Ctx, f.Ids)
	ginx.Dangerous(err)

	re := regexp.MustCompile(`ident\s*=\s*\\".*?\\"`)

	user := c.MustGet("username").(string)
	now := time.Now().Unix()

	newRules := make([]*models.AlertRule, 0)

	reterr := make(map[string]map[string]string)

	for i := range alertRules {
		errMsg := make(map[string]string)

		if alertRules[i].Cate != "prometheus" {
			errMsg["all"] = "Only Prometheus rule can be cloned to machines"
			reterr[alertRules[i].Name] = errMsg
			continue
		}

		if containsIdentOperator(alertRules[i].RuleConfig) {
			errMsg["all"] = "promql is missing ident"
			reterr[alertRules[i].Name] = errMsg
			continue
		}

		for j := range f.IdentList {
			alertRules[i].RuleConfig = re.ReplaceAllString(alertRules[i].RuleConfig, fmt.Sprintf(`ident=\"%s\"`, f.IdentList[j]))

			newRule := &models.AlertRule{}
			if err := copier.Copy(newRule, alertRules[i]); err != nil {
				errMsg[f.IdentList[j]] = fmt.Sprintf("fail to clone rule, err: %s", err)
				continue
			}

			newRule.Id = 0
			newRule.Name = alertRules[i].Name + "_" + f.IdentList[j]
			newRule.CreateBy = user
			newRule.UpdateBy = user
			newRule.UpdateAt = now
			newRule.CreateAt = now
			newRule.RuleConfig = alertRules[i].RuleConfig

			exist, err := models.AlertRuleExists(rt.Ctx, 0, newRule.GroupId, newRule.DatasourceIdsJson, newRule.Name)
			if err != nil {
				errMsg[f.IdentList[j]] = err.Error()
				continue
			}

			if exist {
				errMsg[f.IdentList[j]] = fmt.Sprintf("rule already exists, ruleName: %s", newRule.Name)
				continue
			}

			newRules = append(newRules, newRule)
		}

		if len(errMsg) > 0 {
			reterr[alertRules[i].Name] = errMsg
		}
	}

	ginx.NewRender(c).Data(reterr, models.InsertAlertRule(rt.Ctx, newRules))
}
