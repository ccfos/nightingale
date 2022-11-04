package router

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// 页面上，拉取 server 列表
func serversGet(c *gin.Context) {
	list, err := models.AlertingEngineGets("")
	ginx.NewRender(c).Data(list, err)
}

type serverBindClusterForm struct {
	Cluster string `json:"cluster"`
}

// 用户为某个 n9e-server 分配一个集群，也可以清空，设置cluster为空字符串即可
// 清空就表示这个server没啥用了，可能是要下线掉，或者仅仅用作转发器
func serverBindCluster(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	ae, err := models.AlertingEngineGet("id = ?", id)
	ginx.Dangerous(err)

	if ae == nil {
		ginx.Dangerous("no such server")
	}

	var f serverBindClusterForm
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(ae.UpdateCluster(f.Cluster))
}
