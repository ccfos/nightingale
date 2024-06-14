package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"
)

func (rt *Router) notifyTplGets(c *gin.Context) {
	m := make(map[string]struct{})
	for _, channel := range models.DefaultChannels {
		m[channel] = struct{}{}
	}
	m[models.EmailSubject] = struct{}{}

	lst, err := models.NotifyTplGets(rt.Ctx)
	for i := 0; i < len(lst); i++ {
		if _, exists := m[lst[i].Channel]; exists {
			lst[i].BuiltIn = true
		}
	}

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) notifyTplUpdateContent(c *gin.Context) {
	user := c.MustGet("user").(*models.User)

	var f models.NotifyTpl
	ginx.BindJSON(c, &f)
	ginx.Dangerous(templateValidate(f))

	notifyTpl, err := models.NotifyTplGet(rt.Ctx, f.Id)
	ginx.Dangerous(err)

	if notifyTpl.CreateBy != user.Username && !user.IsAdmin() {
		ginx.Bomb(403, "no permission")
	}

	f.UpdateAt = time.Now().Unix()
	f.UpdateBy = user.Username

	ginx.NewRender(c).Message(f.UpdateContent(rt.Ctx))
}

func (rt *Router) notifyTplUpdate(c *gin.Context) {
	var f models.NotifyTpl
	ginx.BindJSON(c, &f)
	ginx.Dangerous(templateValidate(f))
	user := c.MustGet("user").(*models.User)

	notifyTpl, err := models.NotifyTplGet(rt.Ctx, f.Id)
	ginx.Dangerous(err)

	if notifyTpl.CreateBy != user.Username && !user.IsAdmin() {
		ginx.Bomb(403, "no permission")
	}

	// get the count of the same channel and name but different id
	count, err := models.Count(models.DB(rt.Ctx).Model(&models.NotifyTpl{}).Where("channel = ? or name = ? and id <> ?", f.Channel, f.Name, f.Id))
	ginx.Dangerous(err)
	if count != 0 {
		ginx.Bomb(200, "Refuse to create duplicate channel or name")
	}

	notifyTpl.UpdateAt = time.Now().Unix()
	notifyTpl.UpdateBy = user.Username
	notifyTpl.Name = f.Name

	ginx.NewRender(c).Message(notifyTpl.Update(rt.Ctx))
}

func templateValidate(f models.NotifyTpl) error {
	if len(f.Channel) > 32 {
		return fmt.Errorf("channel length should not exceed 32")
	}

	if str.Dangerous(f.Channel) {
		return fmt.Errorf("channel should not contain dangerous characters")
	}

	if len(f.Name) > 255 {
		return fmt.Errorf("name length should not exceed 255")
	}

	if str.Dangerous(f.Name) {
		return fmt.Errorf("name should not contain dangerous characters")
	}

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
	ginx.Dangerous(err)

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

// add new notify template
func (rt *Router) notifyTplAdd(c *gin.Context) {
	var f models.NotifyTpl
	ginx.BindJSON(c, &f)
	f.Channel = strings.TrimSpace(f.Channel)
	ginx.Dangerous(templateValidate(f))

	count, err := models.Count(models.DB(rt.Ctx).Model(&models.NotifyTpl{}).Where("channel = ? or name = ?", f.Channel, f.Name))
	ginx.Dangerous(err)
	if count != 0 {
		ginx.Bomb(200, "Refuse to create duplicate channel(unique)")
	}
	ginx.NewRender(c).Message(f.Create(rt.Ctx))
}

// delete notify template, not allowed to delete the system defaults(models.DefaultChannels)
func (rt *Router) notifyTplDel(c *gin.Context) {
	f := new(models.NotifyTpl)
	id := ginx.UrlParamInt64(c, "id")
	user := c.MustGet("user").(*models.User)

	notifyTpl, err := models.NotifyTplGet(rt.Ctx, id)
	ginx.Dangerous(err)

	if notifyTpl.CreateBy != user.Username && !user.IsAdmin() {
		ginx.Bomb(403, "no permission")
	}

	ginx.NewRender(c).Message(f.NotifyTplDelete(rt.Ctx, id))
}
