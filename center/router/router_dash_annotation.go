package router

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func checkAnnotationPermission(c *gin.Context, ctx *ctx.Context, dashboardId int64) {
	dashboard, err := models.BoardGetByID(ctx, dashboardId)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "failed to get dashboard: %v", err)
	}

	if dashboard == nil {
		ginx.Bomb(http.StatusNotFound, "dashboard not found")
	}

	bg := BusiGroup(ctx, dashboard.GroupId)
	me := c.MustGet("user").(*models.User)
	can, err := me.CanDoBusiGroup(ctx, bg, "rw")
	ginx.Dangerous(err)

	if !can {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
}

func (rt *Router) dashAnnotationAdd(c *gin.Context) {
	var f models.DashAnnotation
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	now := time.Now().Unix()

	checkAnnotationPermission(c, rt.Ctx, f.DashboardId)

	f.CreateBy = username
	f.CreateAt = now
	f.UpdateBy = username
	f.UpdateAt = now

	ginx.NewRender(c).Data(f.Id, f.Add(rt.Ctx))
}

func (rt *Router) dashAnnotationGets(c *gin.Context) {
	dashboardId := ginx.QueryInt64(c, "dashboard_id")
	from := ginx.QueryInt64(c, "from")
	to := ginx.QueryInt64(c, "to")
	limit := ginx.QueryInt(c, "limit", 100)

	lst, err := models.DashAnnotationGets(rt.Ctx, dashboardId, from, to, limit)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) dashAnnotationPut(c *gin.Context) {
	var f models.DashAnnotation
	ginx.BindJSON(c, &f)

	id := ginx.UrlParamInt64(c, "id")
	annotation, err := getAnnotationById(rt.Ctx, id)
	ginx.Dangerous(err)

	checkAnnotationPermission(c, rt.Ctx, annotation.DashboardId)

	f.Id = id
	f.UpdateAt = time.Now().Unix()
	f.UpdateBy = c.MustGet("username").(string)

	ginx.NewRender(c).Message(f.Update(rt.Ctx))
}

func (rt *Router) dashAnnotationDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	annotation, err := getAnnotationById(rt.Ctx, id)
	ginx.Dangerous(err)
	checkAnnotationPermission(c, rt.Ctx, annotation.DashboardId)

	ginx.NewRender(c).Message(models.DashAnnotationDel(rt.Ctx, id))
}

// 可以提取获取注释的通用方法
func getAnnotationById(ctx *ctx.Context, id int64) (*models.DashAnnotation, error) {
	annotation, err := models.DashAnnotationGet(ctx, "id=?", id)
	if err != nil {
		return nil, err
	}
	if annotation == nil {
		return nil, fmt.Errorf("annotation not found")
	}
	return annotation, nil
}
