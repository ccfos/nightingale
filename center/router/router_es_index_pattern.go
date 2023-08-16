package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// 创建 ES Index Pattern
func (rt *Router) esIndexPatternAdd(c *gin.Context) {
	var f models.EsIndexPattern
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	now := time.Now().Unix()
	f.CreateAt = now
	f.CreateBy = username
	f.UpdateAt = now
	f.UpdateBy = username

	err := f.Add(rt.Ctx)
	ginx.NewRender(c).Message(err)
}

// 更新 ES Index Pattern
func (rt *Router) esIndexPatternPut(c *gin.Context) {
	var f models.EsIndexPattern
	ginx.BindJSON(c, &f)

	id := ginx.QueryInt64(c, "id")

	esIndexPattern, err := models.EsIndexPatternGetById(rt.Ctx, id)
	ginx.Dangerous(err)

	if esIndexPattern == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such EsIndexPattern")
		return
	}

	f.UpdateBy = c.MustGet("username").(string)
	ginx.NewRender(c).Message(esIndexPattern.Update(rt.Ctx, f))
}

// 删除 ES Index Pattern
func (rt *Router) esIndexPatternDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	if len(f.Ids) == 0 {
		ginx.Bomb(http.StatusBadRequest, "ids empty")
	}

	ginx.NewRender(c).Message(models.EsIndexPatternDel(rt.Ctx, f.Ids))
}

// ES Index Pattern列表
func (rt *Router) esIndexPatternGetList(c *gin.Context) {
	datasourceId := ginx.QueryInt64(c, "datasource_id", 0)

	var lst []*models.EsIndexPattern
	var err error
	if datasourceId != 0 {
		lst, err = models.EsIndexPatternGets(rt.Ctx, "datasource_id = ?", datasourceId)
	} else {
		lst, err = models.EsIndexPatternGets(rt.Ctx, "")
	}

	ginx.NewRender(c).Data(lst, err)
}

// ES Index Pattern 单个数据
func (rt *Router) esIndexPatternGet(c *gin.Context) {
	id := ginx.QueryInt64(c, "id")

	item, err := models.EsIndexPatternGet(rt.Ctx, "id=?", id)
	ginx.NewRender(c).Data(item, err)
}
