package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"

	"github.com/didi/nightingale/v5/src/models"
)

// Return all, front-end search and paging
func alertRuleGets(c *gin.Context) {
	busiGroupId := ginx.UrlParamInt64(c, "id")
	ars, err := models.AlertRuleGets(busiGroupId)
	if err == nil {
		cache := make(map[int64]*models.UserGroup)
		for i := 0; i < len(ars); i++ {
			ars[i].FillNotifyGroups(cache)
		}
	}
	ginx.NewRender(c).Data(ars, err)
}

func alertRulesGetByService(c *gin.Context) {
	prods := strings.Split(ginx.QueryStr(c, "prods", ""), ",")
	query := ginx.QueryStr(c, "query", "")
	algorithm := ginx.QueryStr(c, "algorithm", "")
	cluster := ginx.QueryStr(c, "cluster", "")
	cate := ginx.QueryStr(c, "cate", "$all")
	cates := []string{}
	if cate != "$all" {
		cates = strings.Split(cate, ",")
	}

	disabled := ginx.QueryInt(c, "disabled", -1)
	ars, err := models.AlertRulesGetsBy(prods, query, algorithm, cluster, cates, disabled)
	if err == nil {
		cache := make(map[int64]*models.UserGroup)
		for i := 0; i < len(ars); i++ {
			ars[i].FillNotifyGroups(cache)
		}
	}
	ginx.NewRender(c).Data(ars, err)
}

// single or import
func alertRuleAddByFE(c *gin.Context) {
	username := c.MustGet("username").(string)

	var lst []models.AlertRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	bgid := ginx.UrlParamInt64(c, "id")
	reterr := alertRuleAdd(lst, username, bgid, c.GetHeader("X-Language"))

	ginx.NewRender(c).Data(reterr, nil)
}

func alertRuleAddByService(c *gin.Context) {
	var lst []models.AlertRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}
	reterr := alertRuleAddForService(lst, "")
	ginx.NewRender(c).Data(reterr, nil)
}

func alertRuleAddForService(lst []models.AlertRule, username string) map[string]string {
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

		if err := lst[i].Add(); err != nil {
			reterr[lst[i].Name] = err.Error()
		} else {
			reterr[lst[i].Name] = ""
		}
	}
	return reterr
}

func alertRuleAdd(lst []models.AlertRule, username string, bgid int64, lang string) map[string]string {
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

		if err := lst[i].Add(); err != nil {
			reterr[lst[i].Name] = i18n.Sprintf(lang, err.Error())
		} else {
			reterr[lst[i].Name] = ""
		}
	}
	return reterr
}

func alertRuleDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	// param(busiGroupId) for protect
	ginx.NewRender(c).Message(models.AlertRuleDels(f.Ids, ginx.UrlParamInt64(c, "id")))
}

func alertRuleDelByService(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()
	ginx.NewRender(c).Message(models.AlertRuleDels(f.Ids))
}

func alertRulePutByFE(c *gin.Context) {
	var f models.AlertRule
	ginx.BindJSON(c, &f)

	arid := ginx.UrlParamInt64(c, "arid")
	ar, err := models.AlertRuleGetById(arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}

	bgrwCheck(c, ar.GroupId)

	f.UpdateBy = c.MustGet("username").(string)
	ginx.NewRender(c).Message(ar.Update(f))
}

func alertRulePutByService(c *gin.Context) {
	var f models.AlertRule
	ginx.BindJSON(c, &f)

	arid := ginx.UrlParamInt64(c, "arid")
	ar, err := models.AlertRuleGetById(arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}
	ginx.NewRender(c).Message(ar.Update(f))
}

type alertRuleFieldForm struct {
	Ids    []int64                `json:"ids"`
	Fields map[string]interface{} `json:"fields"`
}

// update one field: cluster note severity disabled prom_eval_interval prom_for_duration notify_channels notify_groups notify_recovered notify_repeat_step callbacks runbook_url append_tags
func alertRulePutFields(c *gin.Context) {
	var f alertRuleFieldForm
	ginx.BindJSON(c, &f)

	if len(f.Fields) == 0 {
		ginx.Bomb(http.StatusBadRequest, "fields empty")
	}

	f.Fields["update_by"] = c.MustGet("username").(string)
	f.Fields["update_at"] = time.Now().Unix()

	for i := 0; i < len(f.Ids); i++ {
		ar, err := models.AlertRuleGetById(f.Ids[i])
		ginx.Dangerous(err)

		if ar == nil {
			continue
		}

		ginx.Dangerous(ar.UpdateFieldsMap(f.Fields))
	}

	ginx.NewRender(c).Message(nil)
}

func alertRuleGet(c *gin.Context) {
	arid := ginx.UrlParamInt64(c, "arid")

	ar, err := models.AlertRuleGetById(arid)
	ginx.Dangerous(err)

	if ar == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertRule")
		return
	}

	err = ar.FillNotifyGroups(make(map[int64]*models.UserGroup))
	ginx.NewRender(c).Data(ar, err)
}
