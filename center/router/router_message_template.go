package router

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/slice"
	"github.com/ccfos/nightingale/v6/pkg/strx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/i18n"
)

func (rt *Router) messageTemplatesAdd(c *gin.Context) {
	var lst []*models.MessageTemplate
	ginx.BindJSON(c, &lst)
	if len(lst) == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	me := c.MustGet("user").(*models.User)
	isAdmin := me.IsAdmin()
	idents := make([]string, 0, len(lst))
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	now := time.Now().Unix()
	lang := models.NormalizeMsgTplLang(c.GetHeader("X-Language"))
	for _, tpl := range lst {
		// 生成一个唯一的标识符，以后也不允许修改，前端不需要传这个参数
		tpl.Ident = uuid.New().String()
		// 模板语言由后端按请求语言写入，前端不需要传这个参数
		tpl.Lang = lang

		ginx.Dangerous(tpl.Verify())
		if !isAdmin && !slice.HaveIntersection(gids, tpl.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}
		idents = append(idents, tpl.Ident)

		tpl.CreateBy = me.Username
		tpl.CreateAt = now
		tpl.UpdateBy = me.Username
		tpl.UpdateAt = now
	}

	lstWithSameId, err := models.MessageTemplatesGet(rt.Ctx, "ident IN ?", idents)
	ginx.Dangerous(err)
	if len(lstWithSameId) > 0 {
		ginx.Bomb(http.StatusBadRequest, "ident already exists")
	}

	ids := make([]int64, 0, len(lst))
	for _, tpl := range lst {
		err := models.Insert(rt.Ctx, tpl)
		ginx.Dangerous(err)

		ids = append(ids, tpl.ID)
	}
	ginx.NewRender(c).Data(ids, nil)
}

func (rt *Router) messageTemplatesDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	lst, err := models.MessageTemplatesGet(rt.Ctx, "id in (?)", f.Ids)
	ginx.Dangerous(err)
	notifyRuleIds, err := models.UsedByNotifyRule(rt.Ctx, models.MsgTplList(lst))
	ginx.Dangerous(err)
	if len(notifyRuleIds) > 0 {
		ginx.NewRender(c).Message(fmt.Errorf("used by notify rule: %v", notifyRuleIds))
		return
	}
	if me := c.MustGet("user").(*models.User); !me.IsAdmin() {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		for _, t := range lst {
			if !slice.HaveIntersection(gids, t.UserGroupIds) {
				ginx.Bomb(http.StatusForbidden, "forbidden")
			}
		}
	}

	ginx.NewRender(c).Message(models.DB(rt.Ctx).Delete(
		&models.MessageTemplate{}, "id in (?)", f.Ids).Error)
}

func (rt *Router) messageTemplatePut(c *gin.Context) {
	var f models.MessageTemplate
	ginx.BindJSON(c, &f)

	mt, err := models.MessageTemplateGet(rt.Ctx, "id <> ? and ident = ?", ginx.UrlParamInt64(c, "id"), f.Ident)
	ginx.Dangerous(err)
	if mt != nil {
		ginx.Bomb(http.StatusBadRequest, "message template ident already exists")
	}

	mt, err = models.MessageTemplateGet(rt.Ctx, "id = ?", ginx.UrlParamInt64(c, "id"))
	ginx.Dangerous(err)
	if mt == nil {
		ginx.Bomb(http.StatusNotFound, "message template not found")
	}

	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		if !slice.HaveIntersection(gids, mt.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}
	}

	// 前端编辑时不回传 lang，模板语言保持不变
	f.Lang = mt.Lang
	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(mt.Update(rt.Ctx, f))
}

func (rt *Router) messageTemplateGet(c *gin.Context) {
	me := c.MustGet("user").(*models.User)

	tid := ginx.UrlParamInt64(c, "id")
	mt, err := models.MessageTemplateGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)
	if mt == nil {
		ginx.Bomb(http.StatusNotFound, "message template not found")
	}

	if !me.IsAdmin() && mt.Private == 1 {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		if !slice.HaveIntersection(gids, mt.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}
	}

	ginx.NewRender(c).Data(mt, nil)
}

func (rt *Router) messageTemplatesGet(c *gin.Context) {
	var notifyChannelIdents []string
	if tmp := ginx.QueryStr(c, "notify_channel_idents", ""); tmp != "" {
		notifyChannelIdents = strings.Split(tmp, ",")
	}
	notifyChannelIds := strx.IdsInt64ForAPI(ginx.QueryStr(c, "notify_channel_ids", ""))
	if len(notifyChannelIds) > 0 {
		ginx.Dangerous(models.DB(rt.Ctx).Model(models.NotifyChannelConfig{}).
			Where("id in (?)", notifyChannelIds).Pluck("ident", &notifyChannelIdents).Error)
	}

	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	lst, err := models.MessageTemplatesGetBy(rt.Ctx, notifyChannelIdents)
	ginx.Dangerous(err)
	// 仅对内置模板按语言过滤，用户自建模板始终保留（避免跨语言/存量自建模板被隐藏）
	lst = models.FilterMsgTplsByLang(lst, c.GetHeader("X-Language"))
	models.FillUpdateByNicknames(rt.Ctx, lst)

	if me.IsAdmin() {
		ginx.NewRender(c).Data(lst, nil)
		return
	}

	res := make([]*models.MessageTemplate, 0)
	for _, t := range lst {
		if slice.HaveIntersection[int64](gids, t.UserGroupIds) || t.Private == 0 {
			res = append(res, t)
		}
	}
	ginx.NewRender(c).Data(res, nil)
}

type evtMsgReq struct {
	EventIds []int64 `json:"event_ids"`
	Tpl      struct {
		Content map[string]string `json:"content"`
	} `json:"tpl"`
	// 内嵌而非裸 bool：模板会按 {{if $event.IsRecovered}} 和 severity 分支，
	// 固定样例只能覆盖其中一条路径，用户需要能切级别/恢复态分别预览。
	MockEventForm
}

// previewFieldResult 让「渲染成功的正文」与「模板报错」在类型上可区分。
//
// 改之前两者同为 string，前端把报错当正文渲染：Go 模板错误里的 <.Foo> 片段会被
// dompurify 当非法标签剥掉，用户看到的是一段被截断且看不出是错误的文本。
type previewFieldResult struct {
	Content string `json:"content"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// buildTemplatePreviewMockEvent 构造用于模板预览的内置模拟事件，不落库。
// 与通知规则测试、工作流试跑共用 newMockEvent 骨架。
func buildTemplatePreviewMockEvent(lang string, f MockEventForm) *models.AlertCurEvent {
	return newMockEvent(mockEventSpec{
		RuleName:     i18n.Sprintf(lang, "Message template preview mock event"),
		RuleNote:     i18n.Sprintf(lang, "This is a mock event used by message template preview, it is not persisted"),
		Hash:         "message-template-preview-mock-event",
		Severity:     f.MockSeverity,
		IsRecovered:  f.MockIsRecovered,
		PromQL:       "cpu_usage_active > 80",
		TriggerValue: "81.5",
		ExtraTags:    []string{"ident=mock-host-01", "source=message-template-preview"},
	})
}

func (rt *Router) eventsMessage(c *gin.Context) {
	var req evtMsgReq
	ginx.BindJSON(c, &req)

	var events []*models.AlertCurEvent
	if req.UseMockEvent {
		// 全新环境没有任何历史事件，此前预览必然报 event not found，
		// 导致「建完模板想看看效果」这条路直接断掉。
		events = []*models.AlertCurEvent{buildTemplatePreviewMockEvent(c.GetHeader("X-Language"), req.MockEventForm)}
	} else {
		hisEvents, err := models.AlertHisEventGetByIds(rt.Ctx, req.EventIds)
		ginx.Dangerous(err)

		if len(hisEvents) == 0 {
			ginx.Bomb(http.StatusBadRequest, "event_ids or use_mock_event required")
		}

		events = make([]*models.AlertCurEvent, len(hisEvents))
		for i, he := range hisEvents {
			events[i] = he.ToCur()
			events[i].SetTagsMap()
		}
	}

	renderData := make(map[string]interface{})
	renderData["events"] = events
	// 与 MessageTemplate.RenderEvent 对齐：缺了这个键会让模板里的 {{$.domain}} 预览恒为空
	// （内置模板全都用它拼事件详情链接），而真正发出去的消息里是有值的，预览与实际不一致。
	renderData["domain"] = resolveSiteUrl(rt.Ctx)

	defs := models.GetDefs(renderData)
	ret := make(map[string]previewFieldResult, len(req.Tpl.Content))
	for k, v := range req.Tpl.Content {
		text := strings.Join(append(defs, v), "")
		tpl, err := template.New(k).Funcs(tplx.TemplateFuncMap).Parse(text)
		if err != nil {
			ret[k] = previewFieldResult{Success: false, Message: err.Error()}
			continue
		}

		var buf bytes.Buffer
		if err = tpl.Execute(&buf, renderData); err != nil {
			ret[k] = previewFieldResult{Success: false, Message: err.Error()}
			continue
		}

		ret[k] = previewFieldResult{Success: true, Content: buf.String()}
	}
	ginx.NewRender(c).Data(ret, nil)
}
