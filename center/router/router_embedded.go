package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"strconv"
)

func (rt *Router) embeddedProductGetList(c *gin.Context) {
	dashboards, err := models.ListEmbeddedProducts(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	// 获取当前用户可访问的Group ID 列表
	me := c.MustGet("user").(*models.User)

	if me.IsAdmin() {
		ginx.NewRender(c).Data(dashboards, err)
		return
	}

	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	bgSet := make(map[int64]struct{}, len(gids))
	for _, id := range gids {
		bgSet[id] = struct{}{}
	}
	// 过滤出公开或有权限访问的私有 dashboard
	var result []*models.EmbeddedProduct
	for _, d := range dashboards {
		if !d.IsPrivate {
			result = append(result, d)
		} else {
			for _, tid := range d.TeamIDsJson {
				if _, ok := bgSet[tid]; ok {
					result = append(result, d)
					break
				}
			}
		}
	}
	ginx.NewRender(c).Data(result, err)
}

func (rt *Router) embeddedProductGet(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		ginx.NewRender(c).Message("id is required")
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		ginx.NewRender(c).Message("invalid id")
		return
	}

	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	me := c.MustGet("user").(*models.User)
	ok, err := HasEmbeddedProductAccess(rt.Ctx, me, data)
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	if !ok {
		ginx.NewRender(c, 403).Message("permission denied")
		return
	}
	ginx.NewRender(c).Data(data, nil)
}

func (rt *Router) embeddedProductADD(c *gin.Context) {
	var eps []models.EmbeddedProduct
	ginx.BindJSON(c, &eps)

	me := c.MustGet("user").(*models.User)

	for i := range eps {
		eps[i].CreateBy = me.Nickname
		eps[i].UpdateBy = me.Nickname
	}

	err := models.AddEmbeddedProduct(rt.Ctx, eps)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) embeddedProductPut(c *gin.Context) {
	idStr := c.Param("id")
	var ep models.EmbeddedProduct
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		ginx.NewRender(c).Message("invalid id")
		return
	}
	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	me := c.MustGet("user").(*models.User)
	ok, err := HasEmbeddedProductAccess(rt.Ctx, me, data)

	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	if !ok {
		ginx.NewRender(c, 403).Message("permission denied")
		return
	}
	ep.ID = id
	ginx.BindJSON(c, &ep)

	err = models.UpdateEmbeddedProduct(rt.Ctx, &ep, me.Nickname)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) embeddedProductDelete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		ginx.NewRender(c).Message("invalid id")
		return
	}
	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	me := c.MustGet("user").(*models.User)
	ok, err := HasEmbeddedProductAccess(rt.Ctx, me, data)

	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	if !ok {
		ginx.NewRender(c, 403).Message("permission denied")
		return
	}
	err = models.DeleteEmbeddedProduct(rt.Ctx, id)
	ginx.NewRender(c).Message(err)
}

func HasEmbeddedProductAccess(ctx *ctx.Context, user *models.User, ep *models.EmbeddedProduct) (bool, error) {
	if user.IsAdmin() || !ep.IsPrivate {
		return true, nil
	}

	gids, err := models.MyGroupIds(ctx, user.Id)
	if err != nil {
		return false, err
	}

	groupSet := make(map[int64]struct{}, len(gids))
	for _, gid := range gids {
		groupSet[gid] = struct{}{}
	}

	for _, tid := range ep.TeamIDsJson {
		if _, ok := groupSet[tid]; ok {
			return true, nil
		}
	}

	return false, nil
}
