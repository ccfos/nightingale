package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

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

	if err := templateValidate(f); err != nil {
		ginx.NewRender(c).Message(err.Error())
		return
	}

	ginx.NewRender(c).Message(f.UpdateContent(rt.Ctx))
}

func (rt *Router) notifyTplUpdate(c *gin.Context) {
	var f models.NotifyTpl
	ginx.BindJSON(c, &f)

	if err := templateValidate(f); err != nil {
		ginx.NewRender(c).Message(err.Error())
		return
	}

	ginx.NewRender(c).Message(f.Update(rt.Ctx))
}

func templateValidate(f models.NotifyTpl) error {
	if f.Content == "" {
		return nil
	}

	var defs = []string{
		"{{$labels := .TagsMap}}",
		"{{$value := .TriggerValue}}",
	}
	text := strings.Join(append(defs, f.Content), "")

	if _, err := template.New(f.Channel).Funcs(tplx.TemplateFuncMap).Parse(text); err != nil {
		return fmt.Errorf("notify template verify illegal:%s", err.Error())
	}

	return nil
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

	var defs = []string{
		"{{$labels := .TagsMap}}",
		"{{$value := .TriggerValue}}",
	}
	text := strings.Join(append(defs, f.Content), "")
	tpl, err := template.New(f.Channel).Funcs(tplx.TemplateFuncMap).Parse(text)
	ginx.Dangerous(err)

	event.TagsMap = make(map[string]string)
	for i := 0; i < len(event.TagsJSON); i++ {
		pair := strings.TrimSpace(event.TagsJSON[i])
		if pair == "" {
			continue
		}

		arr := strings.Split(pair, "=")
		if len(arr) != 2 {
			continue
		}

		event.TagsMap[arr[0]] = arr[1]
	}

	var body bytes.Buffer
	var ret string
	if err := tpl.Execute(&body, event); err != nil {
		ret = err.Error()
	} else {
		ret = body.String()
	}

	ginx.NewRender(c).Data(ret, nil)
}
