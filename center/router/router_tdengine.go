package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// {
// 	"cate": "tdengine",
// 	"datasource_id": 1
// }

type databasesQueryForm struct {
	Cate         string `json:"cate" form:"cate"`
	DatasourceId int64  `json:"datasource_id" form:"datasource_id"`
}

// tdengineDatabases
func (rt *Router) tdengineDatabases(c *gin.Context) {
	var f databasesQueryForm
	ginx.BindJSON(c, &f)

	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
	if tdClient == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	databases, err := tdClient.GetDatabases()
	ginx.NewRender(c).Data(databases, err)
}

type tablesQueryForm struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id" `
	Database     string `json:"database"`
	IsTable      bool   `json:"is_table"`
}

// get tdengine tables
func (rt *Router) tdengineTables(c *gin.Context) {
	var f tablesQueryForm
	ginx.BindJSON(c, &f)

	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
	if tdClient == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	tables, err := tdClient.GetTables(f.Database, f.IsTable)
	ginx.NewRender(c).Data(tables, err)
}

type columnsQueryForm struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id" `
	Database     string `json:"database"`
	Table        string `json:"table"`
}

// get tdengine columns
func (rt *Router) tdengineColumns(c *gin.Context) {
	var f columnsQueryForm
	ginx.BindJSON(c, &f)

	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
	if tdClient == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such datasource")
		return
	}

	columns, err := tdClient.GetColumns(f.Database, f.Table)
	ginx.NewRender(c).Data(columns, err)
}
