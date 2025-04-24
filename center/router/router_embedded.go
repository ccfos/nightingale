package router

import (
	"github.com/ccfos/nightingale/v6/models"
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
	me := c.MustGet("user").(*models.User)
	if me.IsAdmin() || data.IsPrivate == false {
		ginx.NewRender(c).Data(data, err)
		return
	}
	// 获取当前用户可访问的Group ID 列表
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	// 构造组 ID 的 Set（用于快速判断）
	groupSet := make(map[int64]struct{})
	for _, gid := range gids {
		groupSet[gid] = struct{}{}
	}

	// 判断是否有交集权限
	for _, tid := range data.TeamIDsJson {
		if _, ok := groupSet[tid]; ok {
			ginx.NewRender(c).Data(data, nil)
			return
		}
	}

	ginx.NewRender(c, 403).Message("permission denied")
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
	ep.ID = id
	ginx.BindJSON(c, &ep)

	me := c.MustGet("user").(*models.User)
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

	err = models.DeleteEmbeddedProduct(rt.Ctx, id)
	ginx.NewRender(c).Message(err)
}
