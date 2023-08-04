package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
)

const (
	ActionAdd    = "callback_add"
	ActionDel    = "callback_del"
	ActionUpdate = "callback_update"
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

	var callbackArray []string

	if vals, ok := f.Fields["callbacks"].([]interface{}); ok {
		// 创建一个新的 []string，将 vals 中的元素转换为 string 类型后存入其中
		callbacks := make([]string, len(vals))
		for i, v := range vals {
			if s, ok := v.(string); ok {
				callbacks[i] = s
			} else {
				// 如果类型断言失败，则输出错误信息
				ginx.Dangerous(fmt.Errorf("invalid type for callbacks: %T", v))
			}
		}
		// 输出 []string
		callbackArray = callbacks
	}

	for i := 0; i < len(f.Ids); i++ {
		ar, err := models.AlertRuleGetById(rt.Ctx, f.Ids[i])
		ginx.Dangerous(err)

		if ar == nil {
			continue
		}

		//TODO: 前端现在 callbacks 给的是 "http://aaaaaa http://bbbbbb" 以空格分割, 现在要求前端给 JSON 数组

		switch f.Action {
		case ActionAdd:
			// 增加一个 callback 地址
			callbackArray = append(ar.Callbacks, callbackArray...)
		case ActionDel:
			// 删除一个 callback 地址
			callbackArray = deleteCallbacks(ar.Callbacks, callbackArray)
		}

		b, err := json.Marshal(callbackArray)
		if err != nil {
			ginx.Dangerous(err)
		}
		f.Fields["callbacks"] = string(b)
		//ginx.Dangerous(ar.UpdateFieldsMap(rt.Ctx, map[string]interface{}{"callbacks": string(b)}))
		for k, v := range f.Fields {
			ginx.Dangerous(ar.UpdateColumn(rt.Ctx, k, v))
		}
	}

	ginx.NewRender(c).Message(nil)
}

// deleteCallbacks 从 callbacks 数组中删除, callback 数组中包含的元素
func deleteCallbacks(callbacks []string, callbackArray []string) []string {
	for _, callback := range callbackArray {
		for i, cb := range callbacks {
			if cb == callback {
				callbacks = append(callbacks[:i], callbacks[i+1:]...)
			}
		}
	}
	return callbacks
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

// pre validation before save rule
func (rt *Router) alertRuleValidation(c *gin.Context) {
	var f models.AlertRule //new
	ginx.BindJSON(c, &f)

	arid := ginx.UrlParamInt64(c, "arid")
	ar, err := models.AlertRuleGetById(rt.Ctx, arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}

	rt.bgrwCheck(c, ar.GroupId)

	if len(f.NotifyChannels) > 0 && len(f.NotifyGroupsJSON) > 0 { //Validation NotifyChannels
		ngids := make([]int64, 0, len(f.NotifyChannels))
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
		ancs := make([]string, 0, len(f.NotifyChannels)) //absent Notify Channels
		for i := range f.NotifyChannels {
			flag := true
			for ui := range users {
				if _, b := users[ui].ExtractToken(f.NotifyChannels[i]); b {
					flag = false
					break
				}
			}
			if flag {
				ancs = append(ancs, f.NotifyChannels[i])
			}
		}

		if len(ancs) > 0 {
			ginx.NewRender(c).Message(i18n.Sprintf(c.GetHeader("X-Language"), "All users are missing notify channel configurations. Please check for missing tokens (each channel should be configured with at least one user). %s", ancs))
			return
		}

	}

	ginx.NewRender(c).Message("")
}
