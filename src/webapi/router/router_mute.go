package router

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

// Return all, front-end search and paging
func alertMuteGetsByBG(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	lst, err := models.AlertMuteGetsByBG(bgid)
	ginx.NewRender(c).Data(lst, err)
}

func alertMuteGets(c *gin.Context) {
	prods := strings.Fields(ginx.QueryStr(c, "prods", ""))
	bgid := ginx.QueryInt64(c, "bgid", 0)
	query := ginx.QueryStr(c, "query", "")
	lst, err := models.AlertMuteGets(prods, bgid, query)

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

func alertMuteAddByService(c *gin.Context) {
	var f models.AlertMute
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(f.Add())
}

func alertMuteDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertMuteDel(f.Ids))
}
