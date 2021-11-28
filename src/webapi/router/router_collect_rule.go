package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func collectRuleGets(c *gin.Context) {
	busiGroupId := ginx.UrlParamInt64(c, "id")
	crs, err := models.CollectRuleGets(busiGroupId, ginx.QueryStr(c, "type", ""))
	ginx.NewRender(c).Data(crs, err)
}

func collectRuleAdd(c *gin.Context) {
	var lst []models.CollectRule
	ginx.BindJSON(c, &lst)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	username := c.MustGet("username").(string)
	bgid := ginx.UrlParamInt64(c, "id")

	// collect rule name -> error string
	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		lst[i].Id = 0
		lst[i].GroupId = bgid
		lst[i].CreateBy = username
		lst[i].UpdateBy = username
		lst[i].FE2DB()

		if err := lst[i].Add(); err != nil {
			reterr[lst[i].Name] = err.Error()
		} else {
			reterr[lst[i].Name] = ""
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}

func collectRuleDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	// param(busiGroupId) for protect
	ginx.NewRender(c).Message(models.CollectRuleDels(f.Ids, ginx.UrlParamInt64(c, "id")))
}

func collectRuleGet(c *gin.Context) {
	crid := ginx.UrlParamInt64(c, "crid")
	cr, err := models.CollectRuleGetById(crid)
	ginx.NewRender(c).Data(cr, err)
}

func collectRulePut(c *gin.Context) {
	var f models.CollectRule
	ginx.BindJSON(c, &f)

	crid := ginx.UrlParamInt64(c, "crid")
	cr, err := models.CollectRuleGetById(crid)
	ginx.Dangerous(err)

	if cr == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such CollectRule")
		return
	}

	f.UpdateBy = c.MustGet("username").(string)
	ginx.NewRender(c).Message(cr.Update(f))
}
