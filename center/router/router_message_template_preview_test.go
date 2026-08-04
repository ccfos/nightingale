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

// 站点地址走渲染数据而不是 defs 变量：内置模板与前端生成的起步内容都写
// {{$.domain}}。此前预览接口没填 renderData["domain"]，这些模板在预览里恒为空、
// 与真正发出去的消息对不上；这里确认补上的那个键真的能被模板取到。
func TestEventsMessagePreviewResolvesDomain(t *testing.T) {
	events := []*models.AlertCurEvent{
		buildTemplatePreviewMockEvent("en_US", MockEventForm{MockSeverity: 2}),
	}
	renderData := map[string]interface{}{
		"events": events,
		"domain": "http://site",
	}

	t.Run("$.domain 能取到预览接口填的站点地址", func(t *testing.T) {
		got := renderPreviewField(t, "{{$.domain}}/alert-his-events/{{$event.Id}}", renderData)
		if !got.Success {
			t.Fatalf("preview failed: %s", got.Message)
		}
		if got.Content != "http://site/alert-his-events/0" {
			t.Fatalf("content = %q", got.Content)
		}
	})

	// 裸 {{$domain}} 不是本系统提供的变量（它只在老的 notify_tpl 内置模板里由模板
	// 自行声明），预览必须如实报错，而不是靠 defs 悄悄兜住——否则开源能预览、
	// 覆盖了 GetDefs 的下游发出去才炸。
	t.Run("裸 $domain 仍然报未定义变量", func(t *testing.T) {
		got := renderPreviewField(t, "{{$domain}}", renderData)
		if got.Success {
			t.Fatalf("expected failure, got %q", got.Content)
		}
		if !strings.Contains(got.Message, "undefined variable") {
			t.Fatalf("message = %q", got.Message)
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
