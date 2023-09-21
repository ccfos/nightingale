package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

type databasesQueryForm struct {
	Cate         string `json:"cate" form:"cate"`
	DatasourceId int64  `json:"datasource_id" form:"datasource_id"`
}

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
	Database     string `json:"db"`
	IsStable     bool   `json:"is_stable"`
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

	tables, err := tdClient.GetTables(f.Database, f.IsStable)
	ginx.NewRender(c).Data(tables, err)
}

type columnsQueryForm struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id" `
	Database     string `json:"db"`
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

func (rt *Router) QueryData(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	var resp []*models.DataResp
	var err error
	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
	for _, q := range f.Querys {
		datas, err := tdClient.Query(q)
		ginx.Dangerous(err)
		resp = append(resp, datas...)
	}

	ginx.NewRender(c).Data(resp, err)
}

func (rt *Router) QueryLog(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	tdClient := rt.TdendgineClients.GetCli(f.DatasourceId)
	if len(f.Querys) == 0 {
		ginx.Bomb(200, "querys is empty")
		return
	}

	data, err := tdClient.QueryLog(f.Querys[0])
	logger.Debugf("tdengine query:%s result: %+v", f.Querys[0], data)
	ginx.NewRender(c).Data(data, err)
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
