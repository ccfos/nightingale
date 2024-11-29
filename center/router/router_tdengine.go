package router

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/ds/tdengine"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"net/http"
)

type databasesQueryForm struct {
	Cate         string `json:"cate" form:"cate"`
	DatasourceId int64  `json:"datasource_id" form:"datasource_id"`
}

func (rt *Router) tdengineDatabases(c *gin.Context) {
	var f databasesQueryForm
	ginx.BindJSON(c, &f)

	datasource, hit := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if _, ok := datasource.(*tdengine.TDengine); !hit || !ok {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	databases, err := datasource.(*tdengine.TDengine).ShowDatabases(rt.Ctx.Ctx)
	ginx.NewRender(c).Data(databases, err)
}

type tablesQueryForm struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id" `
	Database     string `json:"db"`
	IsStable     bool   `json:"is_stable"`
}

// get tdengine tables
func (rt *Router) tdengineTables(c *gin.Context) {
	var f tablesQueryForm
	ginx.BindJSON(c, &f)

	datasource, hit := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if _, ok := datasource.(*tdengine.TDengine); !hit || !ok {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	database := fmt.Sprintf("%s.tables", f.Database)
	if f.IsStable {
		database = fmt.Sprintf("%s.stables", database)
	}

	tables, err := datasource.(*tdengine.TDengine).ShowTables(rt.Ctx.Ctx, database)
	ginx.NewRender(c).Data(tables, err)
}

type columnsQueryForm struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id" `
	Database     string `json:"db"`
	Table        string `json:"table"`
}

func (rt *Router) tdengineColumns(c *gin.Context) {
	var f columnsQueryForm
	ginx.BindJSON(c, &f)

	datasource, hit := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if _, ok := datasource.(*tdengine.TDengine); !hit || !ok {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	query := map[string]string{
		"database": f.Database,
		"table":    f.Table,
	}

	columns, err := datasource.(*tdengine.TDengine).DescribeTable(rt.Ctx.Ctx, query)
	ginx.NewRender(c).Data(columns, err)
}

// query sql template
func (rt *Router) QuerySqlTemplate(c *gin.Context) {
	cate := ginx.QueryStr(c, "cate")
	m := make(map[string]string)
	switch cate {
	case models.TDENGINE:
		m = cconf.TDengineSQLTpl
	}
	ginx.NewRender(c).Data(m, nil)
}
