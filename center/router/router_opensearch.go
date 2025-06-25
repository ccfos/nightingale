package router

import (
	"github.com/ccfos/nightingale/v6/datasource/opensearch"
	"github.com/ccfos/nightingale/v6/dscache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func (rt *Router) QueryOSIndices(c *gin.Context) {
	var f IndexReq
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	indices, err := plug.(*opensearch.OpenSearch).QueryIndices()
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(indices, nil)
}

func (rt *Router) QueryOSFields(c *gin.Context) {
	var f IndexReq
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	fields, err := plug.(*opensearch.OpenSearch).QueryFields([]string{f.Index})
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(fields, nil)
}

func (rt *Router) QueryOSVariable(c *gin.Context) {
	var f FieldValueReq
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	fields, err := plug.(*opensearch.OpenSearch).QueryFieldValue([]string{f.Index}, f.Query.Field, f.Query.Query)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(fields, nil)
}
