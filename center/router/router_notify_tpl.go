package router

import (
	"bytes"
	"encoding/json"
	"html/template"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) notifyTplGets(c *gin.Context) {
	lst, err := models.NotifyTplGets(rt.Ctx)

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) notifyTplUpdateContent(c *gin.Context) {
	var f models.NotifyTpl
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(f.UpdateContent(rt.Ctx))
}

func (rt *Router) notifyTplUpdate(c *gin.Context) {
	var f models.NotifyTpl
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(f.Update(rt.Ctx))
}

func (rt *Router) notifyTplPreview(c *gin.Context) {
	var event models.AlertCurEvent
	err := json.Unmarshal([]byte(cconf.EVENT_EXAMPLE), &event)
	if err != nil {
		ginx.NewRender(c).Message(err.Error())
		return
	}

	var f models.NotifyTpl
	ginx.BindJSON(c, &f)

	tpl, err := template.New(f.Channel).Funcs(tplx.TemplateFuncMap).Parse(f.Content)
	ginx.Dangerous(err)

	var body bytes.Buffer
	var ret string
	if err := tpl.Execute(&body, event); err != nil {
		ret = err.Error()
	} else {
		ret = body.String()
	}

	ginx.NewRender(c).Data(ret, nil)
}
