package router

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

// 复刻 eventsMessage 的渲染管线（handler 本身要 DB：resolveSiteUrl -> ConfigsGetSiteUrl，
// 这里只钉住 defs + Parse + Execute 这一段，与 handler 保持逐行一致）。
func renderPreviewField(t *testing.T, body string, renderData map[string]interface{}) previewFieldResult {
	t.Helper()

	defs := models.GetDefs(renderData)
	text := strings.Join(append(defs, body), "")
	tpl, err := template.New("k").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return previewFieldResult{Success: false, Message: err.Error()}
	}
	var buf bytes.Buffer
	if err = tpl.Execute(&buf, renderData); err != nil {
		return previewFieldResult{Success: false, Message: err.Error()}
	}
	return previewFieldResult{Success: true, Content: buf.String()}
}

// getDefs 是模板预览与真实投递共用的变量声明。$domain 少了会让预览直接报
// undefined variable，用户在预览里看到的是一句 Go 报错而不是渲染结果；
// 预览接口特意填了 renderData["domain"]，这里确认它真的能被模板取到。
func TestEventsMessagePreviewResolvesDomain(t *testing.T) {
	events := []*models.AlertCurEvent{
		buildTemplatePreviewMockEvent("en_US", MockEventForm{MockSeverity: 2}),
	}
	renderData := map[string]interface{}{
		"events": events,
		"domain": "http://site",
	}

	t.Run("$domain 能取到预览接口填的站点地址", func(t *testing.T) {
		got := renderPreviewField(t, "{{$domain}}/alert-his-events/{{$event.Id}}", renderData)
		if !got.Success {
			t.Fatalf("preview failed: %s", got.Message)
		}
		if got.Content != "http://site/alert-his-events/0" {
			t.Fatalf("content = %q", got.Content)
		}
	})

	// 模拟事件必须让 $labels 可用，文档示例里到处都是它
	t.Run("$labels / $value 仍然可用", func(t *testing.T) {
		got := renderPreviewField(t, "{{$value}}|{{len $labels}}", renderData)
		if !got.Success {
			t.Fatalf("preview failed: %s", got.Message)
		}
		if !strings.HasPrefix(got.Content, "81.5|") {
			t.Fatalf("content = %q", got.Content)
		}
	})

	// 真正写错的模板仍要走 Success=false，不能被上面的兜底吞掉
	t.Run("语法错误仍然报告为失败", func(t *testing.T) {
		got := renderPreviewField(t, "{{$nope}}", renderData)
		if got.Success {
			t.Fatalf("expected failure, got %q", got.Content)
		}
		if !strings.Contains(got.Message, "undefined variable") {
			t.Fatalf("message = %q", got.Message)
		}
	})
}
