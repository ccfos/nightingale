package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

func (rt *Router) embeddedProductGets(c *gin.Context) {
	products, err := models.EmbeddedProductGets(rt.Ctx)
	ginx.Dangerous(err)
	models.FillUpdateByNicknames(rt.Ctx, products)
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
		eps[i].CreateBy = me.Username
		eps[i].UpdateBy = me.Username
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
	oldProduct.Hide = ep.Hide
	oldProduct.TeamIDs = ep.TeamIDs
	oldProduct.Weight = ep.Weight
	oldProduct.UpdateBy = me.Username
	oldProduct.UpdateAt = now

	err = models.UpdateEmbeddedProduct(rt.Ctx, oldProduct)
	ginx.NewRender(c).Message(err)
}

// embeddedProductHidePut 单独更新 hide 字段，供"显示 / 隐藏"开关使用。
// 请求体：{"hide": true}
func (rt *Router) embeddedProductHidePut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	if id <= 0 {
		ginx.Bomb(400, "invalid id")
	}

	var req struct {
		Hide bool `json:"hide"`
	}
	ginx.BindJSON(c, &req)

	me := c.MustGet("user").(*models.User)
	err := models.UpdateEmbeddedProductHide(rt.Ctx, id, req.Hide, me.Username)
	ginx.NewRender(c).Message(err)
}

// embeddedProductWeightsPut 批量更新 weight，供前端拖拽排序使用。
// 请求体：[{"id": 1, "weight": 0}, {"id": 2, "weight": 1}, ...]
func (rt *Router) embeddedProductWeightsPut(c *gin.Context) {
	var items []struct {
		ID     int64 `json:"id"`
		Weight int   `json:"weight"`
	}
	ginx.BindJSON(c, &items)

	// 上限保护：避免单个事务内跑过多 UPDATE 造成长事务/锁表
	const maxBatchSize = 1000
	if len(items) > maxBatchSize {
		ginx.Bomb(400, "too many items")
	}

	weights := make(map[int64]int, len(items))
	for _, it := range items {
		if it.ID <= 0 {
			ginx.Bomb(400, "invalid id")
		}
		weights[it.ID] = it.Weight
	}

	me := c.MustGet("user").(*models.User)
	err := models.UpdateEmbeddedProductWeights(rt.Ctx, weights, me.Username)
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
