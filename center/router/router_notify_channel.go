package router

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) notifyChannelsAdd(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	var lst []*models.NotifyChannelConfig
	ginx.BindJSON(c, &lst)
	if len(lst) == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	idents := make([]string, 0, len(lst))
	for _, tpl := range lst {
		ginx.Dangerous(tpl.Verify())
		idents = append(idents, tpl.Ident)

		tpl.CreateBy = me.Username
		tpl.CreateAt = time.Now().Unix()
		tpl.UpdateBy = me.Username
		tpl.UpdateAt = time.Now().Unix()
	}
	lstWithSameId, err := models.NotifyChannelsGet(rt.Ctx, "ident IN ?", idents)
	ginx.Dangerous(err)
	if len(lstWithSameId) > 0 {
		ginx.Bomb(http.StatusBadRequest, "ident already exists")
	}

	ginx.Dangerous(models.DB(rt.Ctx).CreateInBatches(lst, 100).Error)
	ids := make([]uint, 0, len(lst))
	for _, tpl := range lst {
		ids = append(ids, tpl.ID)
	}
	ginx.NewRender(c).Data(ids, nil)
}

func (rt *Router) notifyChannelsDel(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	lst, err := models.NotifyChannelsGet(rt.Ctx, "id in (?)", f.Ids)
	ginx.Dangerous(err)
	notifyRuleIds, err := models.UsedByNotifyRule(rt.Ctx, models.NotiChList(lst))
	ginx.Dangerous(err)
	if len(notifyRuleIds) > 0 {
		ginx.NewRender(c).Message(fmt.Errorf("used by notify rule: %v", notifyRuleIds))
		return
	}

	ginx.NewRender(c).Message(models.DB(rt.Ctx).
		Delete(&models.NotifyChannelConfig{}, "id in (?)", f.Ids).Error)
}

func (rt *Router) notifyChannelPut(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	var f models.NotifyChannelConfig
	ginx.BindJSON(c, &f)

	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", ginx.UrlParamInt64(c, "id"))
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(nc.Update(rt.Ctx, f))
}

func (rt *Router) notifyChannelGet(c *gin.Context) {
	tid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	ginx.NewRender(c).Data(nc, nil)
}

func (rt *Router) notifyChannelsGet(c *gin.Context) {
	lst, err := models.NotifyChannelsGet(rt.Ctx, "", nil)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) notifyChannelsGetForNormalUser(c *gin.Context) {
	lst, err := models.NotifyChannelsGet(rt.Ctx, "", nil)
	ginx.Dangerous(err)
	newLst := make([]*models.NotifyChannelConfig, 0, len(lst))
	for _, c := range lst {
		newLst = append(newLst, &models.NotifyChannelConfig{
			Name:        c.Name,
			Ident:       c.Ident,
			ParamConfig: c.ParamConfig,
		})
	}
	ginx.NewRender(c).Data(newLst, nil)
}
