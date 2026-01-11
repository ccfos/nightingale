package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/slice"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) savedViewGets(c *gin.Context) {
	page := ginx.QueryStr(c, "page", "")

	me := c.MustGet("user").(*models.User)

	lst, err := models.SavedViewGets(rt.Ctx, page)
	if err != nil {
		ginx.NewRender(c).Data(nil, err)
		return
	}

	userGids, err := models.MyGroupIds(rt.Ctx, me.Id)
	if err != nil {
		ginx.NewRender(c).Data(nil, err)
		return
	}

	favoriteMap, err := models.SavedViewFavoriteGetByUserId(rt.Ctx, me.Id)
	if err != nil {
		ginx.NewRender(c).Data(nil, err)
		return
	}

	favoriteViews := make([]models.SavedView, 0)
	normalViews := make([]models.SavedView, 0)

	for _, view := range lst {
		visible := view.CreateBy == me.Username ||
			view.PublicCate == 2 ||
			(view.PublicCate == 1 && slice.HaveIntersection[int64](userGids, view.Gids))

		if !visible {
			continue
		}

		view.IsFavorite = favoriteMap[view.Id]

		// 收藏的排前面
		if view.IsFavorite {
			favoriteViews = append(favoriteViews, view)
		} else {
			normalViews = append(normalViews, view)
		}
	}

	ginx.NewRender(c).Data(append(favoriteViews, normalViews...), nil)
}

func (rt *Router) savedViewAdd(c *gin.Context) {
	var f models.SavedView
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	f.Id = 0
	f.CreateBy = me.Username
	f.UpdateBy = me.Username

	err := models.SavedViewAdd(rt.Ctx, &f)
	ginx.NewRender(c).Data(f.Id, err)
}

func (rt *Router) savedViewPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	view, err := models.SavedViewGetById(rt.Ctx, id)
	if err != nil {
		ginx.NewRender(c).Data(nil, err)
		return
	}
	if view == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("saved view not found")
		return
	}

	me := c.MustGet("user").(*models.User)
	// 只有创建者可以更新
	if view.CreateBy != me.Username && !me.IsAdmin() {
		ginx.NewRender(c, http.StatusForbidden).Message("forbidden")
		return
	}

	var f models.SavedView
	ginx.BindJSON(c, &f)

	view.Name = f.Name
	view.Filter = f.Filter
	view.PublicCate = f.PublicCate
	view.Gids = f.Gids

	err = models.SavedViewUpdate(rt.Ctx, view, me.Username)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) savedViewDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	view, err := models.SavedViewGetById(rt.Ctx, id)
	if err != nil {
		ginx.NewRender(c).Data(nil, err)
		return
	}
	if view == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("saved view not found")
		return
	}

	me := c.MustGet("user").(*models.User)
	// 只有创建者或管理员可以删除
	if view.CreateBy != me.Username && !me.IsAdmin() {
		ginx.NewRender(c, http.StatusForbidden).Message("forbidden")
		return
	}

	err = models.SavedViewDel(rt.Ctx, id)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) savedViewFavoriteAdd(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	me := c.MustGet("user").(*models.User)

	err := models.UserViewFavoriteAdd(rt.Ctx, id, me.Id)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) savedViewFavoriteDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	me := c.MustGet("user").(*models.User)

	err := models.UserViewFavoriteDel(rt.Ctx, id, me.Id)
	ginx.NewRender(c).Message(err)
}
