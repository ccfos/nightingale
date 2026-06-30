package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/datasource/iotdb"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
)

func (rt *Router) iotdbDatabases(c *gin.Context) {
	var f databasesQueryForm
	ginx.BindJSON(c, &f)

	datasource, hit := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if _, ok := datasource.(*iotdb.IoTDB); !hit || !ok {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	databases, err := datasource.(*iotdb.IoTDB).ShowDatabases(rt.Ctx.Ctx)
	ginx.NewRender(c).Data(databases, err)
}

func (rt *Router) iotdbTables(c *gin.Context) {
	var f tablesQueryForm
	ginx.BindJSON(c, &f)

	datasource, hit := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if _, ok := datasource.(*iotdb.IoTDB); !hit || !ok {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	tables, err := datasource.(*iotdb.IoTDB).ShowTables(rt.Ctx.Ctx, f.Database)
	ginx.NewRender(c).Data(tables, err)
}

func (rt *Router) iotdbColumns(c *gin.Context) {
	var f columnsQueryForm
	ginx.BindJSON(c, &f)

	datasource, hit := dscache.DsCache.Get(f.Cate, f.DatasourceId)
	if _, ok := datasource.(*iotdb.IoTDB); !hit || !ok {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	query := map[string]string{
		"database": f.Database,
		"table":    f.Table,
	}

	columns, err := datasource.(*iotdb.IoTDB).DescribeTable(rt.Ctx.Ctx, query)
	iotColumns := make([]Column, len(columns))
	for i, column := range columns {
		iotColumns[i] = Column{
			Name: column.Field,
			Type: column.Type,
		}
	}
	ginx.NewRender(c).Data(iotColumns, err)
}
