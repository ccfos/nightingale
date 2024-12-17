package router

import (
	"github.com/ccfos/nightingale/v6/datasource/es"
	"github.com/ccfos/nightingale/v6/dscache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

type IndexReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Index        string `json:"index"`
}

type FieldValueReq struct {
	Cate         string   `json:"cate"`
	DatasourceId int64    `json:"datasource_id"`
	Index        string   `json:"index"`
	Query        FieldObj `json:"query"`
}

type FieldObj struct {
	Find  string `json:"find"`
	Field string `json:"field"`
	Query string `json:"query"`
}

func (rt *Router) QueryIndices(c *gin.Context) {
	var f IndexReq
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	indices, err := plug.(*es.Elasticsearch).QueryIndices()
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(indices, nil)
}

func (rt *Router) QueryFields(c *gin.Context) {
	var f IndexReq
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	fields, err := plug.(*es.Elasticsearch).QueryFields([]string{f.Index})
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(fields, nil)
}

func (rt *Router) QueryESVariable(c *gin.Context) {
	var f FieldValueReq
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	fields, err := plug.(*es.Elasticsearch).QueryFieldValue([]string{f.Index}, f.Query.Field, f.Query.Query)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(fields, nil)
}
