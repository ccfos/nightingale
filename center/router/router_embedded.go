package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) embeddedProductGets(c *gin.Context) {
	products, err := models.EmbeddedProductGets(rt.Ctx)
	ginx.Dangerous(err)
	// 获取当前用户可访问的Group ID 列表
	me := c.MustGet("user").(*models.User)

	if me.IsAdmin() {
		ginx.NewRender(c).Data(products, err)
		return
	}

	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	bgSet := make(map[int64]struct{}, len(gids))
	for _, id := range gids {
		bgSet[id] = struct{}{}
	}

	// 过滤出公开或有权限访问的私有 product link
	var result []*models.EmbeddedProduct
	for _, product := range products {
		if !product.IsPrivate {
			result = append(result, product)
			continue
		}

		for _, tid := range product.TeamIDs {
			if _, ok := bgSet[tid]; ok {
				result = append(result, product)
				break
			}
		}
	}

	ginx.NewRender(c).Data(result, err)
}

func (rt *Router) embeddedProductGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	if id <= 0 {
		ginx.Bomb(400, "invalid id")
	}

	data, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	ginx.Dangerous(err)

	me := c.MustGet("user").(*models.User)
	hashPermission, err := hasEmbeddedProductAccess(rt.Ctx, me, data)
	ginx.Dangerous(err)

	if !hashPermission {
		ginx.Bomb(403, "forbidden")
	}

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
	id := ginx.UrlParamInt64(c, "id")
	ginx.BindJSON(c, &ep)

	if id <= 0 {
		ginx.Bomb(400, "invalid id")
	}

	oldProduct, err := models.GetEmbeddedProductByID(rt.Ctx, id)
	ginx.Dangerous(err)
	me := c.MustGet("user").(*models.User)

	now := time.Now().Unix()
	oldProduct.Name = ep.Name
	oldProduct.URL = ep.URL
	oldProduct.IsPrivate = ep.IsPrivate
	oldProduct.TeamIDs = ep.TeamIDs
	oldProduct.UpdateBy = me.Username
	oldProduct.UpdateAt = now

	err = models.UpdateEmbeddedProduct(rt.Ctx, oldProduct)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) embeddedProductDelete(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	if id <= 0 {
		ginx.Bomb(400, "invalid id")
	}

	err := models.DeleteEmbeddedProduct(rt.Ctx, id)
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
