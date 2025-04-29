package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) embeddedProductGetList(c *gin.Context) {
	dashboards, err := models.ListEmbeddedProducts(rt.Ctx)
	ginx.Dangerous(err)
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
			for _, tid := range d.TeamIDs {
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
	idInt64 := ginx.UrlParamInt64(c, "id")
	if idInt64 <= 0 {
		ginx.NewRender(c).Message("invalid id")
		return
	}
	id := uint64(idInt64)

	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	ginx.Dangerous(err)
	me := c.MustGet("user").(*models.User)
	ok, err := hasEmbeddedProductAccess(rt.Ctx, me, data)
	ginx.Dangerous(err)
	ginx.Dangerous(ok, 403)
	ginx.NewRender(c).Data(data, nil)
}

func (rt *Router) embeddedProductAdd(c *gin.Context) {
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
	var ep models.EmbeddedProduct
	idInt64 := ginx.UrlParamInt64(c, "id")
	if idInt64 <= 0 {
		ginx.NewRender(c).Message("invalid id")
		return
	}
	id := uint64(idInt64)
	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	ginx.Dangerous(err)
	me := c.MustGet("user").(*models.User)
	ok, err := hasEmbeddedProductAccess(rt.Ctx, me, data)
	ginx.Dangerous(err)
	ginx.Dangerous(ok, 403)

	ep.ID = id
	ginx.BindJSON(c, &ep)

	err = models.UpdateEmbeddedProduct(rt.Ctx, &ep, me.Nickname)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) embeddedProductDelete(c *gin.Context) {
	idInt64 := ginx.UrlParamInt64(c, "id")
	if idInt64 <= 0 {
		ginx.NewRender(c).Message("invalid id")
		return
	}
	id := uint64(idInt64)
	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	ginx.Dangerous(err)
	me := c.MustGet("user").(*models.User)
	ok, err := hasEmbeddedProductAccess(rt.Ctx, me, data)
	ginx.Dangerous(err)
	ginx.Dangerous(ok, 403)
	err = models.DeleteEmbeddedProduct(rt.Ctx, id)
	ginx.NewRender(c).Message(err)
}

func hasEmbeddedProductAccess(ctx *ctx.Context, user *models.User, ep *models.EmbeddedProduct) (bool, error) {
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

	for _, tid := range ep.TeamIDs {
		if _, ok := groupSet[tid]; ok {
			return true, nil
		}
	}

	return false, nil
}
