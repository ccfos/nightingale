package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type busiGroupForm struct {
	Name        string                   `json:"name" binding:"required"`
	LabelEnable int                      `json:"label_enable"`
	LabelValue  string                   `json:"label_value"`
	Members     []models.BusiGroupMember `json:"members"`
}

func (rt *Router) busiGroupAdd(c *gin.Context) {
	var f busiGroupForm
	ginx.BindJSON(c, &f)

	if len(f.Members) == 0 {
		ginx.Bomb(http.StatusBadRequest, "members empty")
	}

	rwhas := false
	for i := 0; i < len(f.Members); i++ {
		if f.Members[i].PermFlag == "rw" {
			rwhas = true
			break
		}
	}

	if !rwhas {
		ginx.Bomb(http.StatusBadRequest, "At least one team have rw permission")
	}

	username := c.MustGet("username").(string)
	ginx.Dangerous(models.BusiGroupAdd(rt.Ctx, f.Name, f.LabelEnable, f.LabelValue, f.Members, username))

	// 如果创建成功，拿着name去查，应该可以查到
	newbg, err := models.BusiGroupGet(rt.Ctx, "name=?", f.Name)
	ginx.Dangerous(err)

	if newbg == nil {
		ginx.NewRender(c).Message("Failed to create BusiGroup(%s)", f.Name)
		return
	}

	ginx.NewRender(c).Data(newbg.Id, nil)
}

func (rt *Router) busiGroupPut(c *gin.Context) {
	var f busiGroupForm
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	targetbg := c.MustGet("busi_group").(*models.BusiGroup)
	ginx.NewRender(c).Message(targetbg.Update(rt.Ctx, f.Name, f.LabelEnable, f.LabelValue, username))
}

func (rt *Router) busiGroupMemberAdd(c *gin.Context) {
	var members []models.BusiGroupMember
	ginx.BindJSON(c, &members)

	username := c.MustGet("username").(string)
	targetbg := c.MustGet("busi_group").(*models.BusiGroup)

	for i := 0; i < len(members); i++ {
		if members[i].BusiGroupId != targetbg.Id {
			ginx.Bomb(http.StatusBadRequest, "business group id invalid")
		}
	}

	ginx.NewRender(c).Message(targetbg.AddMembers(rt.Ctx, members, username))
}

func (rt *Router) busiGroupMemberDel(c *gin.Context) {
	var members []models.BusiGroupMember
	ginx.BindJSON(c, &members)

	username := c.MustGet("username").(string)
	targetbg := c.MustGet("busi_group").(*models.BusiGroup)

	for i := 0; i < len(members); i++ {
		if members[i].BusiGroupId != targetbg.Id {
			ginx.Bomb(http.StatusBadRequest, "business group id invalid")
		}
	}

	ginx.NewRender(c).Message(targetbg.DelMembers(rt.Ctx, members, username))
}

func (rt *Router) busiGroupDel(c *gin.Context) {
	username := c.MustGet("username").(string)
	targetbg := c.MustGet("busi_group").(*models.BusiGroup)

	err := targetbg.Del(rt.Ctx)
	if err != nil {
		logger.Infof("busi_group_delete fail: operator=%s, group_name=%s error=%v", username, targetbg.Name, err)
	} else {
		logger.Infof("busi_group_delete succ: operator=%s, group_name=%s", username, targetbg.Name)
	}

	ginx.NewRender(c).Message(err)
}

// 我是超管、或者我是业务组成员
func (rt *Router) busiGroupGets(c *gin.Context) {
	limit := ginx.QueryInt(c, "limit", defaultLimit)
	query := ginx.QueryStr(c, "query", "")
	all := ginx.QueryBool(c, "all", false)

	me := c.MustGet("user").(*models.User)
	lst, err := me.BusiGroups(rt.Ctx, limit, query, all)
	if len(lst) == 0 {
		lst = []models.BusiGroup{}
	}

	ginx.NewRender(c).Data(lst, err)
}

// 这个接口只有在活跃告警页面才调用，获取各个BG的活跃告警数量
func (rt *Router) busiGroupAlertingsGets(c *gin.Context) {
	ids := ginx.QueryStr(c, "ids", "")
	ret, err := models.AlertNumbers(rt.Ctx, str.IdsInt64(ids))
	ginx.NewRender(c).Data(ret, err)
}

func (rt *Router) busiGroupGet(c *gin.Context) {
	bg := BusiGroup(rt.Ctx, ginx.UrlParamInt64(c, "id"))
	ginx.Dangerous(bg.FillUserGroups(rt.Ctx))
	ginx.NewRender(c).Data(bg, nil)
}
