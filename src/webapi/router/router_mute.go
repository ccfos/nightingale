package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

// Return all, front-end search and paging
func alertMuteGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	lst, err := models.AlertMuteGets(bgid)
	ginx.NewRender(c).Data(lst, err)
}

func alertMuteAdd(c *gin.Context) {
	var f models.AlertMute
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	f.CreateBy = username
	f.GroupId = ginx.UrlParamInt64(c, "id")

	ginx.NewRender(c).Message(f.Add())
}

func alertMuteDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertMuteDel(f.Ids))
}
