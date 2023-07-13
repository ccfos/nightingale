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
	// 判定datasource_id 和 name 是否已经存在
	existsIndexPatterns, err := models.EsIndexPatternGets(rt.Ctx, "datasource_id = ? and name = ?", f.DatasourceId, f.Name)
	ginx.Dangerous(err)

	var oldEsIndexPattern *models.EsIndexPattern
	for _, indexPattern := range existsIndexPatterns {
		if indexPattern.Id != id {
			ginx.Bomb(http.StatusOK, "es index pattern datasource and name already exists")
		} else {
			oldEsIndexPattern = indexPattern
		}
	}

	if oldEsIndexPattern == nil {
		ginx.Bomb(http.StatusOK, "es index pattern not found")
	}

	oldEsIndexPattern.Name = f.Name
	oldEsIndexPattern.DatasourceId = f.DatasourceId
	oldEsIndexPattern.TimeField = f.TimeField
	oldEsIndexPattern.HideSystemIndices = f.HideSystemIndices
	oldEsIndexPattern.FieldsFormat = f.FieldsFormat

	username := c.MustGet("username").(string)
	oldEsIndexPattern.UpdateBy = username
	oldEsIndexPattern.UpdateAt = time.Now().Unix()

	ginx.NewRender(c).Message(oldEsIndexPattern.Update(rt.Ctx, "datasource_id", "name", "time_field", "hide_system_indices", "fields_format", "update_at", "update_by"))
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
func (rt *Router) esIndexPatternGetByDs(c *gin.Context) {
	datasourceId := ginx.QueryInt64(c, "datasource_id")

	lst, err := models.EsIndexPatternGets(rt.Ctx, "datasource_id = ?", datasourceId)
	ginx.NewRender(c).Data(lst, err)
}

// ES Index Pattern 单个数据
func (rt *Router) esIndexPatternGet(c *gin.Context) {
	id := ginx.QueryInt64(c, "id")

	item, err := models.EsIndexPatternGet(rt.Ctx, "id=?", id)
	ginx.NewRender(c).Data(item, err)
}
