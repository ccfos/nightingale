package http

import (
	"github.com/didi/nightingale/v4/src/modules/server/ssoc"

	"github.com/gin-gonic/gin"
)

func ssoClientsPost(c *gin.Context) {
	ssoc.CreateClient(c.Writer, c.Request.Body)
}

func ssoClientsGet(c *gin.Context) {
	c.Request.ParseForm()
	ssoc.GetClients(c.Writer, c.Request.Form)
}

func ssoClientGet(c *gin.Context) {
	clientId := urlParamStr(c, "clientId")
	ssoc.GetClient(c.Writer, clientId)
}

func ssoClientPut(c *gin.Context) {
	clientId := urlParamStr(c, "clientId")

	ssoc.UpdateClient(c.Writer, clientId, c.Request.Body)
}

func ssoClientDel(c *gin.Context) {
	clientId := urlParamStr(c, "clientId")
	ssoc.DeleteClient(c.Writer, clientId)
}
