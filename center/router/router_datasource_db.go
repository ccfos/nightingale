package router

import (
	"context"
	"net/http"

	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/gin-gonic/gin"
)

type DatasourceMetaHookFunc func(c *gin.Context, request *models.QueryParam, response []string) ([]string, error)

var DatasourceMetaHook DatasourceMetaHookFunc = func(c *gin.Context, request *models.QueryParam, response []string) ([]string, error) {
	if response == nil {
		return make([]string, 0), nil
	}
	return response, nil
}

type DatasourceDescribeHookFunc func(c *gin.Context, request *models.QueryParam) error

var DatasourceDescribeHook DatasourceDescribeHookFunc = func(c *gin.Context, request *models.QueryParam) error {
	return nil
}

func (rt *Router) ShowDatabases(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", f.DatasourceId)
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

	databases, hookErr := DatasourceMetaHook(c, &f, databases)
	if hookErr != nil {
		ginx.Bomb(http.StatusForbidden, "%s", hookErr.Error())
	}

	ginx.NewRender(c).Data(databases, nil)
}

func (rt *Router) ShowTables(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", f.DatasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	type TableShower interface {
		ShowTables(ctx context.Context, database string) ([]string, error)
	}
	shower, ok := plug.(TableShower)
	if !ok {
		ginx.Bomb(200, "datasource not exists")
	}

	// 只接受一个入参
	if len(f.Queries) == 0 {
		ginx.NewRender(c).Data(make([]string, 0), nil)
		return
	}
	database, ok := f.Queries[0].(string)
	if !ok {
		ginx.NewRender(c).Data(make([]string, 0), nil)
		return
	}

	tables, err := shower.ShowTables(c.Request.Context(), database)
	if err != nil {
		ginx.NewRender(c).Data(tables, err)
		return
	}

	tables, hookErr := DatasourceMetaHook(c, &f, tables)
	if hookErr != nil {
		ginx.Bomb(http.StatusForbidden, "%s", hookErr.Error())
	}

	ginx.NewRender(c).Data(tables, nil)
}

func (rt *Router) DescribeTable(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", f.DatasourceId)
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
		if len(f.Queries) > 0 {
			if hookErr := DatasourceDescribeHook(c, &f); hookErr != nil {
				ginx.Bomb(http.StatusForbidden, "%s", hookErr.Error())
			}
			columns, err = client.DescribeTable(c.Request.Context(), f.Queries[0])
		}
	default:
		ginx.Bomb(200, "datasource not exists")
	}

	ginx.NewRender(c).Data(columns, err)
}
