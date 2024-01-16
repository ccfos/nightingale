package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) serversGet(c *gin.Context) {
	list, err := models.AlertingEngineGets(rt.Ctx, "")
	ginx.NewRender(c).Data(list, err)
}

func (rt *Router) serverClustersGet(c *gin.Context) {
	list, err := models.AlertingEngineGetsClusters(rt.Ctx, "")
	ginx.NewRender(c).Data(list, err)
}

func (rt *Router) serverHeartbeat(c *gin.Context) {
	var req models.HeartbeatInfo
	ginx.BindJSON(c, &req)
	err := models.AlertingEngineHeartbeatWithCluster(rt.Ctx, req.Instance, req.EngineCluster, req.DatasourceId)
	ginx.NewRender(c).Message(err)
}

func (rt *Router) serversActive(c *gin.Context) {
	datasourceId := ginx.QueryInt64(c, "dsid", 0)
	engineName := ginx.QueryStr(c, "engine_name", "")
	if engineName != "" {
		servers, err := models.AlertingEngineGetsInstances(rt.Ctx, "engine_cluster = ? and clock > ?", engineName, time.Now().Unix()-30)
		ginx.NewRender(c).Data(servers, err)
		return
	}

	if datasourceId == 0 {
		ginx.NewRender(c).Message("dsid is required")
		return
	}
	servers, err := models.AlertingEngineGetsInstances(rt.Ctx, "datasource_id = ? and clock > ?", datasourceId, time.Now().Unix()-30)
	ginx.NewRender(c).Data(servers, err)
}
