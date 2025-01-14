package router

import (
	"context"

	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func (rt *Router) ShowDatabases(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	var databases []string
	var err error
	type DatabaseShower interface {
		ShowDatabases(context.Context) ([]string, error)
	}
	switch plug.(type) {
	case DatabaseShower:
		databases, err = plug.(DatabaseShower).ShowDatabases(c.Request.Context())
		ginx.Dangerous(err)
	default:
		ginx.Bomb(200, "datasource not exists")
	}

	if len(databases) == 0 {
		databases = make([]string, 0)
	}

	ginx.NewRender(c).Data(databases, nil)
}

func (rt *Router) ShowTables(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	// 只接受一个入参
	tables := make([]string, 0)
	var err error
	type TableShower interface {
		ShowTables(ctx context.Context, database string) ([]string, error)
	}
	switch plug.(type) {
	case TableShower:
		if len(f.Querys) > 0 {
			database, ok := f.Querys[0].(string)
			if ok {
				tables, err = plug.(TableShower).ShowTables(c.Request.Context(), database)
			}
		}
	default:
		ginx.Bomb(200, "datasource not exists")
	}
	ginx.NewRender(c).Data(tables, err)
}

func (rt *Router) DescribeTable(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logger.Warningf("cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}
	// 只接受一个入参
	columns := make([]*types.ColumnProperty, 0)
	var err error
	type TableDescriber interface {
		DescribeTable(context.Context, interface{}) ([]*types.ColumnProperty, error)
	}
	switch plug.(type) {
	case TableDescriber:
		client := plug.(TableDescriber)
		if len(f.Querys) > 0 {
			columns, err = client.DescribeTable(c.Request.Context(), f.Querys[0])
		}
	default:
		ginx.Bomb(200, "datasource not exists")
	}

	ginx.NewRender(c).Data(columns, err)
}
