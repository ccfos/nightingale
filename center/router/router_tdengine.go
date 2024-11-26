package router

import (
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

//type databasesQueryForm struct {
//	Cate         string `json:"cate" form:"cate"`
//	DatasourceId int64  `json:"datasource_id" form:"datasource_id"`
//}

//func (rt *Router) tdengineDatabases(c *gin.Context) {
//	var f databasesQueryForm
//	ginx.BindJSON(c, &f)
//
//	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
//	if tdClient == nil {
//		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
//		return
//	}
//
//	databases, err := tdClient.GetDatabases()
//	ginx.NewRender(c).Data(databases, err)
//}

//type tablesQueryForm struct {
//	Cate         string `json:"cate"`
//	DatasourceId int64  `json:"datasource_id" `
//	Database     string `json:"db"`
//	IsStable     bool   `json:"is_stable"`
//}

//// get tdengine tables
//func (rt *Router) tdengineTables(c *gin.Context) {
//	var f tablesQueryForm
//	ginx.BindJSON(c, &f)
//
//	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
//	if tdClient == nil {
//		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
//		return
//	}
//
//	tables, err := tdClient.GetTables(f.Database, f.IsStable)
//	ginx.NewRender(c).Data(tables, err)
//}
//
//type columnsQueryForm struct {
//	Cate         string `json:"cate"`
//	DatasourceId int64  `json:"datasource_id" `
//	Database     string `json:"db"`
//	Table        string `json:"table"`
//}

// get tdengine columns
//func (rt *Router) tdengineColumns(c *gin.Context) {
//	var f columnsQueryForm
//	ginx.BindJSON(c, &f)
//
//	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
//	if tdClient == nil {
//		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
//		return
//	}
//
//	columns, err := tdClient.GetColumns(f.Database, f.Table)
//	ginx.NewRender(c).Data(columns, err)
//}

// xub todo 这个接口实现在这里保留？
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
