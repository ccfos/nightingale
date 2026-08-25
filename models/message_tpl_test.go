package models

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

// 与 eventsMessage 渲染链路一致：拼上 $event 等变量定义后，各语言模板都应能正常解析
func TestNewTplMapLangVariantsParse(t *testing.T) {
	for lang, m := range map[string]map[string]string{"en": NewTplMapEn, "ja": NewTplMapJa} {
		for key, text := range m {
			full := strings.Join(append(getDefs(nil), text), "")
			if _, err := texttemplate.New(key).Funcs(tplx.TemplateFuncMap).Parse(full); err != nil {
				t.Errorf("built-in %s template %s parse error: %v", lang, key, err)
			}
		}
	}
}

// tplActionRe 匹配模板动作 {{...}}
var tplActionRe = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

// TestNewTplMapJaKeepsEnActions 日文模板是从英文版逐字替换标签文案生成的，
// 两者的模板动作序列必须完全一致：只要有一处 {{...}} 被翻译动作误伤，
// 渲染结果就会静默丢字段（模板仍能解析，只是取不到值）
func TestNewTplMapJaKeepsEnActions(t *testing.T) {
	if len(NewTplMapJa) != len(NewTplMapEn)+1 { // 多一条日文 EmailSubject
		t.Fatalf("NewTplMapJa has %d entries, NewTplMapEn has %d", len(NewTplMapJa), len(NewTplMapEn))
	}

	for key, enText := range NewTplMapEn {
		jaText, ok := NewTplMapJa[key]
		if !ok {
			t.Errorf("NewTplMapJa missing %s", key)
			continue
		}

		enActions := tplActionRe.FindAllString(enText, -1)
		jaActions := tplActionRe.FindAllString(jaText, -1)
		if len(enActions) != len(jaActions) {
			t.Errorf("%s: ja has %d template actions, en has %d", key, len(jaActions), len(enActions))
			continue
		}
		for i := range enActions {
			if enActions[i] != jaActions[i] {
				t.Errorf("%s: action %d differs\n en: %s\n ja: %s", key, i, enActions[i], jaActions[i])
			}
		}
	}
}

// TestNewTplMapJaHasNoLeftoverLabels 生成脚本漏译时标签会原样留在日文模板里，
// 这里抽查几个高频标签兜底
func TestNewTplMapJaHasNoLeftoverLabels(t *testing.T) {
	labels := []string{"Level Status", "Rule Name", "Trigger Time", "Recovery Time", "Send Time", "Trigger Value"}
	for key, text := range NewTplMapJa {
		stripped := tplActionRe.ReplaceAllString(text, "")
		for _, label := range labels {
			if strings.Contains(stripped, label) {
				t.Errorf("ja template %s still contains untranslated label %q", key, label)
			}
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

// 内置模板各语言版本与中文版一一对应：ident 为中文版 ident 加语言后缀，
// 渠道一致，内容 key 一致。新增语言时在这里加一行即可
func TestMsgTplMapLangVariantsMirrorMsgTplMap(t *testing.T) {
	variants := []struct {
		lang   string
		suffix string
		tpls   []MessageTemplate
	}{
		{MsgTplLangEn, "-en", MsgTplMapEn},
		{MsgTplLangJa, "-ja", MsgTplMapJa},
	}

	zhByIdent := make(map[string]MessageTemplate, len(MsgTplMap))
	for _, tpl := range MsgTplMap {
		zhByIdent[tpl.Ident] = tpl
	}

	for _, v := range variants {
		if len(v.tpls) != len(MsgTplMap) {
			t.Errorf("%s variant has %d templates, MsgTplMap has %d", v.lang, len(v.tpls), len(MsgTplMap))
			continue
		}

		for _, tpl := range v.tpls {
			if tpl.Lang != v.lang {
				t.Errorf("built-in %s template %s lang = %q, want %q", v.lang, tpl.Ident, tpl.Lang, v.lang)
			}

			if tpl.Ident != tpl.NotifyChannelIdent+v.suffix {
				t.Errorf("built-in %s template ident %q should be %q", v.lang, tpl.Ident, tpl.NotifyChannelIdent+v.suffix)
			}

			zhTpl, ok := zhByIdent[tpl.NotifyChannelIdent]
			if !ok {
				t.Errorf("built-in %s template %s has no zh counterpart", v.lang, tpl.Ident)
				continue
			}

			for key := range zhTpl.Content {
				if _, ok := tpl.Content[key]; !ok {
					t.Errorf("built-in %s template %s missing content key %q", v.lang, tpl.Ident, key)
				}
			}
		}
	}
}

// TestBuiltinMsgTplIdentsUnique 三套内置模板共用 message_template.ident 唯一键，
// 后缀写错会让 Upsert 互相覆盖，落库后只剩一门语言
func TestBuiltinMsgTplIdentsUnique(t *testing.T) {
	seen := make(map[string]string)
	for name, tpls := range map[string][]MessageTemplate{
		"zh": MsgTplMap, "en": MsgTplMapEn, "ja": MsgTplMapJa,
	} {
		for _, tpl := range tpls {
			if prev, dup := seen[tpl.Ident]; dup {
				t.Errorf("ident %q used by both %s and %s", tpl.Ident, prev, name)
			}
			seen[tpl.Ident] = name
		}
	}
}

// TestBuiltinMsgTplLangMatchesHeader 种子行的 Lang 必须等于 NormalizeMsgTplLang
// 对该语言请求头的返回值，否则 FilterMsgTplsByLang 永远筛不出这套模板
func TestBuiltinMsgTplLangMatchesHeader(t *testing.T) {
	cases := map[string][]MessageTemplate{
		"en_US": MsgTplMapEn,
		"ja_JP": MsgTplMapJa,
	}
	for header, tpls := range cases {
		want := NormalizeMsgTplLang(header)
		for _, tpl := range tpls {
			if got := NormalizeMsgTplLang(tpl.Lang); got != want {
				t.Errorf("template %s: NormalizeMsgTplLang(%q) = %q, header %q normalizes to %q",
					tpl.Ident, tpl.Lang, got, header, want)
			}
		}
	}
}

// 站点地址对外公开的写法是 {{$.domain}}（前端字段面板与文档都按这个给），
// 它走渲染数据查找、不经过 getDefs，因此不受 GetDefs 被下游覆盖的影响。
// 这里把这条契约钉住：既要保证 $.domain 在任意位置都取得到，也要保证
// getDefs 不会偷偷再引入一个 $domain 变量——那会造成「开源能用、覆盖了
// GetDefs 的下游不能用」的静默分叉。
func TestDomainComesFromRenderDataNotDefs(t *testing.T) {
	renderData := map[string]interface{}{
		"events": []*AlertCurEvent{{RuleName: "r1", TagsMap: map[string]string{"env": "prod"}}},
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

	t.Run("模板可直接引用 $.domain", func(t *testing.T) {
		if got := render(t, "{{$.domain}}/alert-his-events/{{$event.Id}}", renderData); got != "http://site/alert-his-events/0" {
			t.Fatalf("got %q", got)
		}
	})

	// 对外只推荐 $.domain 而不是 .domain，正是因为后者在 range/with 里的 dot 已被改写。
	// 字段面板里的表达式是「点一下就复制粘贴到任意位置」的，必须在任意位置都成立。
	t.Run("range 内部 $.domain 仍然成立", func(t *testing.T) {
		body := "{{range $k, $v := $event.TagsMap}}[{{$.domain}}]{{end}}"
		if got := render(t, body, renderData); got != "[http://site]" {
			t.Fatalf("got %q", got)
		}
	})

	// getDefs 不得声明 $domain：内置模板（notify_tpl.go 与 plus 的 modelx）都是
	// 先 {{$domain := "..."}} 自己声明再用，一旦这里也声明，能不能用就取决于
	// GetDefs 有没有被覆盖，而覆盖方是照抄一份、不会跟着同步。
	t.Run("getDefs 不声明 $domain", func(t *testing.T) {
		for _, def := range getDefs(renderData) {
			if strings.Contains(def, "$domain") {
				t.Fatalf("getDefs 声明了 $domain: %s\n"+
					"这会制造静默分叉：GetDefs 是可被下游覆盖的函数变量，覆盖方（如 n9e-plus 的 "+
					"modelx.GetDefs）是照抄一份而非在基线上追加，不会跟着同步。结果是同一份模板"+
					"在开源版能解析、在覆盖了 GetDefs 的部署里 Parse 报 undefined variable，"+
					"而 RenderEvent 把解析错误当正文发给第三方。\n"+
					"站点地址请用渲染数据里的 {{$.domain}}（不经过 defs，两边天然一致）；"+
					"确需新增模板变量时，请先让下游改成在基线上追加。", def)
			}
		}
	})

	// 自己声明 $domain 的存量模板不受影响
	t.Run("模板内部自行声明 $domain 仍然可用", func(t *testing.T) {
		if got := render(t, `{{$domain := "http://custom" }}{{$domain}}`, renderData); got != "http://custom" {
			t.Fatalf("got %q", got)
		}
	})
}

// 渲染分支由 NotifyChannelIdent 决定，而不是由调用方碰巧传了什么。
//
// 「保存前测试媒介配置」是拿请求体里的模板源码现构造一个 MessageTemplate 去渲染的，
// 一旦忘了带 ident 就会一律落到默认分支：测试邮件正文里的换行变成字面量 \n、引号变成 \"，
// 而保存之后走生产链路（模板行带 notify_channel_ident）却是正常的——同一份模板两种结果，
// 用户只会以为自己模板写错了。
func TestRenderEventBranchDependsOnNotifyChannelIdent(t *testing.T) {
	events := []*AlertCurEvent{{RuleName: "r1", TagsMap: map[string]string{}}}
	// 正文里同时有换行和引号：默认分支会把它们转义掉，email 分支必须原样保留
	content := map[string]string{"content": "line1\nsay \"hi\""}

	t.Run("email 走 text/template 且不转义", func(t *testing.T) {
		got := (&MessageTemplate{Content: content, NotifyChannelIdent: "email"}).RenderEvent(events, "http://site")
		if want := "line1\nsay \"hi\""; fmt.Sprint(got["content"]) != want {
			t.Fatalf("got %q, want %q", fmt.Sprint(got["content"]), want)
		}
	})

	// 这一条正是漏传 ident 时会发生的事，留在这里说明「默认分支不是无害兜底」
	t.Run("ident 为空落到默认分支，换行与引号被转义", func(t *testing.T) {
		got := (&MessageTemplate{Content: content}).RenderEvent(events, "http://site")
		if want := `line1\nsay \"hi\"`; fmt.Sprint(got["content"]) != want {
			t.Fatalf("got %q, want %q", fmt.Sprint(got["content"]), want)
		}
	})

	t.Run("slack 分支把 &lt; 还原成 <", func(t *testing.T) {
		slack := map[string]string{"content": "<http://x|link>"}
		got := (&MessageTemplate{Content: slack, NotifyChannelIdent: "slackwebhook"}).RenderEvent(events, "http://site")
		if s := fmt.Sprint(got["content"]); !strings.Contains(s, "<http://x|link>") {
			t.Fatalf("got %q", s)
		}
	})
}

// RenderEvent 把模板错误当正文返回（生产链路的既有行为，不改），
// RenderEventStrict 必须往外抛——否则「保存前测试」会在模板写错时报成功，
// 而第三方收到的是一段 "failed to parse template: ..."。
func TestRenderEventStrictReportsTemplateErrors(t *testing.T) {
	events := []*AlertCurEvent{{RuleName: "r1", TagsMap: map[string]string{}}}
	broken := map[string]string{"content": "{{$nope}}"}

	t.Run("RenderEvent 把错误当正文返回", func(t *testing.T) {
		got := (&MessageTemplate{Content: broken}).RenderEvent(events, "http://site")
		if s := fmt.Sprint(got["content"]); !strings.Contains(s, "failed to parse template") {
			t.Fatalf("既有吞错行为被改变了，got %q", s)
		}
	})

	t.Run("RenderEventStrict 返回 error 且带字段名", func(t *testing.T) {
		got, err := (&MessageTemplate{Content: broken}).RenderEventStrict(events, "http://site")
		if err == nil {
			t.Fatalf("expected error, got content %v", got)
		}
		if got != nil {
			t.Fatalf("出错时不应返回半成品内容: %v", got)
		}
		if !strings.Contains(err.Error(), `"content"`) || !strings.Contains(err.Error(), "undefined variable") {
			t.Fatalf("错误信息要能定位到字段与原因，got %q", err.Error())
		}
	})

	// 执行期错误（Parse 过得去、Execute 挂）同样要报
	t.Run("执行期错误也报", func(t *testing.T) {
		bad := map[string]string{"content": "{{index $events 5}}"}
		if _, err := (&MessageTemplate{Content: bad}).RenderEventStrict(events, "http://site"); err == nil {
			t.Fatal("expected execute error")
		}
	})

	t.Run("模板全部正确时与 RenderEvent 结果一致", func(t *testing.T) {
		ok := map[string]string{"title": "{{$event.RuleName}}", "content": "{{$.domain}}"}
		strict, err := (&MessageTemplate{Content: ok}).RenderEventStrict(events, "http://site")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		loose := (&MessageTemplate{Content: ok}).RenderEvent(events, "http://site")
		if fmt.Sprint(strict) != fmt.Sprint(loose) {
			t.Fatalf("strict=%v loose=%v", strict, loose)
		}
	})

	// slack 分支历来是把出错字段整个丢掉而不是写入错误文本，重构不能改掉这个差异
	t.Run("slack 分支出错时仍然丢字段", func(t *testing.T) {
		got := (&MessageTemplate{Content: broken, NotifyChannelIdent: "slackbot"}).RenderEvent(events, "http://site")
		if _, ok := got["content"]; ok {
			t.Fatalf("slack 分支应丢掉出错字段，got %v", got)
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
