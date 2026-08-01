package models

import (
	"bytes"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

// 与 eventsMessage 渲染链路一致：拼上 $event 等变量定义后，英文模板都应能正常解析
func TestNewTplMapEnParse(t *testing.T) {
	for key, text := range NewTplMapEn {
		full := strings.Join(append(getDefs(nil), text), "")
		if _, err := texttemplate.New(key).Funcs(tplx.TemplateFuncMap).Parse(full); err != nil {
			t.Errorf("built-in en template %s parse error: %v", key, err)
		}
	}
}

func TestNormalizeMsgTplLang(t *testing.T) {
	cases := map[string]string{
		"":      "",
		"zh":    "",
		"zh_CN": "",
		"zh_HK": "",
		"en":    MsgTplLangEn,
		"en_US": MsgTplLangEn,
		"ja_JP": "ja_JP",
	}

	for in, want := range cases {
		if got := NormalizeMsgTplLang(in); got != want {
			t.Errorf("NormalizeMsgTplLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterMsgTplsByLang(t *testing.T) {
	sysZh := &MessageTemplate{ID: 1, Lang: "", CreateBy: "system"}
	sysEn := &MessageTemplate{ID: 2, Lang: "en", CreateBy: "system"}
	userZh := &MessageTemplate{ID: 3, Lang: "", CreateBy: "root"}    // 存量/中文侧自建
	userEn := &MessageTemplate{ID: 4, Lang: "en", CreateBy: "alice"} // 英文侧自建
	userJa := &MessageTemplate{ID: 5, Lang: "ja_JP", CreateBy: "bob"}
	all := []*MessageTemplate{sysZh, sysEn, userZh, userEn, userJa}

	tests := []struct {
		name    string
		reqLang string
		lst     []*MessageTemplate
		wantIds []int64
	}{
		// 内置模板按语言过滤，自建模板始终保留、与请求语言无关
		{"中文请求：中文内置 + 全部自建", "zh_CN", all, []int64{1, 3, 4, 5}},
		{"zh_HK 同样按中文处理", "zh_HK", all, []int64{1, 3, 4, 5}},
		{"未携带 X-Language 按中文处理", "", all, []int64{1, 3, 4, 5}},
		{"英文请求：英文内置 + 全部自建（含 lang 为空的存量自建）", "en_US", all, []int64{2, 3, 4, 5}},
		{"其他语言无内置模板时内置回退英文，自建全保留", "ru_RU", all, []int64{2, 3, 4, 5}},
		{"英文请求但英文内置缺失时回退中文内置", "en_US", []*MessageTemplate{sysZh, userZh}, []int64{1, 3}},
		{"其他语言且英文内置也缺失时回退中文内置", "ja_JP", []*MessageTemplate{sysZh, userJa}, []int64{1, 5}},
		{"仅自建模板时与语言无关全部返回", "en", []*MessageTemplate{userZh, userJa}, []int64{3, 5}},
	}

	for _, tt := range tests {
		got := FilterMsgTplsByLang(tt.lst, tt.reqLang)
		gotIds := make([]int64, 0, len(got))
		for _, tpl := range got {
			gotIds = append(gotIds, tpl.ID)
		}

		if len(gotIds) != len(tt.wantIds) {
			t.Errorf("%s: FilterMsgTplsByLang(%q) got ids %v, want %v", tt.name, tt.reqLang, gotIds, tt.wantIds)
			continue
		}
		for i := range gotIds {
			if gotIds[i] != tt.wantIds[i] {
				t.Errorf("%s: FilterMsgTplsByLang(%q) got ids %v, want %v", tt.name, tt.reqLang, gotIds, tt.wantIds)
				break
			}
		}
	}
}

// 内置模板中英文一一对应：英文版 ident 为中文版 ident 加 -en 后缀，渠道一致，内容 key 一致
func TestMsgTplMapEnMirrorsMsgTplMap(t *testing.T) {
	zhByIdent := make(map[string]MessageTemplate, len(MsgTplMap))
	for _, tpl := range MsgTplMap {
		zhByIdent[tpl.Ident] = tpl
	}

	if len(MsgTplMapEn) != len(MsgTplMap) {
		t.Fatalf("MsgTplMapEn has %d templates, MsgTplMap has %d", len(MsgTplMapEn), len(MsgTplMap))
	}

	for _, enTpl := range MsgTplMapEn {
		if enTpl.Lang != MsgTplLangEn {
			t.Errorf("built-in en template %s lang = %q, want %q", enTpl.Ident, enTpl.Lang, MsgTplLangEn)
		}

		if enTpl.Ident != enTpl.NotifyChannelIdent+"-en" {
			t.Errorf("built-in en template ident %q should be %q", enTpl.Ident, enTpl.NotifyChannelIdent+"-en")
		}

		zhTpl, ok := zhByIdent[enTpl.NotifyChannelIdent]
		if !ok {
			t.Errorf("built-in en template %s has no zh counterpart", enTpl.Ident)
			continue
		}

		for key := range zhTpl.Content {
			if _, ok := enTpl.Content[key]; !ok {
				t.Errorf("built-in en template %s missing content key %q", enTpl.Ident, key)
			}
		}
	}
}

// $domain 是文档与前端字段面板对外公开的变量，必须由 getDefs 声明。
// 少了它，任何写 {{$domain}} 的模板会在 Parse 阶段报 undefined variable，
// 而 RenderEvent 把解析错误当正文返回——第三方收到的是一段
// "failed to parse template: ..."，且媒介测试仍报成功，错误只在真实群消息里才看得见。
func TestGetDefsProvidesDomain(t *testing.T) {
	renderData := map[string]interface{}{
		"events": []*AlertCurEvent{{RuleName: "r1", TagsMap: map[string]string{}}},
		"domain": "http://site",
	}

	render := func(t *testing.T, body string, data map[string]interface{}) string {
		t.Helper()
		full := strings.Join(append(getDefs(data), body), "")
		tpl, err := texttemplate.New("k").Funcs(tplx.TemplateFuncMap).Parse(full)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			t.Fatalf("execute error: %v", err)
		}
		return buf.String()
	}

	t.Run("模板可直接引用 $domain", func(t *testing.T) {
		if got := render(t, "{{$domain}}/alert-his-events/{{$event.Id}}", renderData); got != "http://site/alert-his-events/0" {
			t.Fatalf("got %q", got)
		}
	})

	// 内置模板（notify_tpl.go）都是先 {{$domain := "..."}} 再用，后声明者必须仍然胜出，
	// 否则这次改动会把所有内置模板里的域名替换掉
	t.Run("模板内部重新赋值仍然生效", func(t *testing.T) {
		if got := render(t, `{{$domain := "http://custom" }}{{$domain}}`, renderData); got != "http://custom" {
			t.Fatalf("got %q", got)
		}
	})

	// 将来若有调用方漏填 domain，应渲染成空串而不是报错
	t.Run("缺 domain 键时渲染为空且不报错", func(t *testing.T) {
		data := map[string]interface{}{"events": renderData["events"]}
		if got := render(t, "[{{$domain}}]", data); got != "[]" {
			t.Fatalf("got %q", got)
		}
	})
}

// 中文内置模板同样要能在加上 getDefs 之后解析（英文版由 TestNewTplMapEnParse 覆盖）
func TestNewTplMapParse(t *testing.T) {
	for key, text := range NewTplMap {
		full := strings.Join(append(getDefs(nil), text), "")
		if _, err := texttemplate.New(key).Funcs(tplx.TemplateFuncMap).Parse(full); err != nil {
			t.Errorf("built-in template %s parse error: %v", key, err)
		}
	}
}
