package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"":      LangSource,
		"zh":    LangSource,
		"zh_CN": LangSource,
		"zh_HK": LangSource,
		"en":    LangEnUS,
		"en_US": LangEnUS,
		"en_GB": LangEnUS,
		"ja_JP": LangEnUS, // 未支持语言归入 en_US
		"ru_RU": LangEnUS,
		// 大小写不敏感：第三方直连 API 可能传大写
		"ZH_CN": LangSource,
		"Zh-CN": LangSource,
		"ZH":    LangSource,
		"EN_US": LangEnUS,
	}
	for in, want := range cases {
		if got := NormalizeLang(in); got != want {
			t.Errorf("NormalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTranslateAlertMap(t *testing.T) {
	dict := ComponentDict{
		"磁盘使用率过高": "Disk usage too high",
		"触发值":     "trigger value",
		"描述":      "Description",
	}

	m := map[string]interface{}{
		"name": "磁盘使用率过高",
		"note": "未收录的说明", // 词条缺失，应保留原文（单字段回退）
		"annotations": map[string]interface{}{
			"summary": "触发值", // 值翻译
			"描述":      "触发值", // key 与值都是展示内容，都译
			"极值":      "触发值", // key 无词条时保留原文
		},
		"rule_config": map[string]interface{}{
			"queries": []interface{}{
				map[string]interface{}{"prom_ql": "disk_used_percent > 90"},
			},
		},
	}
	TranslateAlertMap(m, dict)

	if m["name"] != "Disk usage too high" {
		t.Errorf("name = %v", m["name"])
	}
	if m["note"] != "未收录的说明" {
		t.Errorf("note should fall back to source text, got %v", m["note"])
	}
	ann := m["annotations"].(map[string]interface{})
	if ann["summary"] != "trigger value" || ann["极值"] != "trigger value" {
		t.Errorf("annotations = %v", ann)
	}
	if _, ok := ann["描述"]; ok {
		t.Errorf("annotation key 描述 should be translated away, got %v", ann)
	}
	if ann["Description"] != "trigger value" {
		t.Errorf("annotation key should be translated to Description, got %v", ann)
	}
}

// 告警 annotations 的 key 与 value 都参与翻译，但 key 冲突时保留原 key，
// 避免条目被覆盖丢失
func TestTranslateAlertMapAnnotationKeyCollision(t *testing.T) {
	dict := ComponentDict{"描述": "Description", "说明": "Description"}
	m := map[string]interface{}{
		"annotations": map[string]interface{}{"描述": "a", "说明": "b"},
	}
	TranslateAlertMap(m, dict)
	ann := m["annotations"].(map[string]interface{})
	if len(ann) != 2 {
		t.Fatalf("annotation entries lost on key collision: %v", ann)
	}
}

func TestTranslateDashboardMapWhitelist(t *testing.T) {
	// 同一字符串同时出现在 var.name（功能标识）与 panels.name（展示字段），
	// 只有后者被替换
	dict := ComponentDict{
		"内存":     "Memory",
		"大盘名":    "Board Name",
		"每秒查询":   "QPS",
		"重命名列":   "Renamed Column",
		"阿里云 RDS": "AliYun RDS",
	}

	m := map[string]interface{}{
		"name": "大盘名",
		"tags": "阿里云 RDS",
		"configs": map[string]interface{}{
			"var": []interface{}{
				map[string]interface{}{"name": "内存", "label": "内存"},
			},
			"panels": []interface{}{
				map[string]interface{}{
					"name": "内存",
					"targets": []interface{}{
						map[string]interface{}{"legend": "每秒查询", "expr": "rate(x[1m])"},
					},
					"transformations": []interface{}{
						map[string]interface{}{
							"options": map[string]interface{}{
								"renameByName": map[string]interface{}{"col_a": "重命名列"},
							},
						},
					},
					// 嵌套面板
					"panels": []interface{}{
						map[string]interface{}{"name": "内存", "description": "未收录描述"},
					},
				},
			},
		},
	}
	TranslateDashboardMap(m, dict)

	if m["name"] != "Board Name" {
		t.Errorf("board name = %v", m["name"])
	}
	if m["tags"] != "AliYun RDS" {
		t.Errorf("board tags = %v", m["tags"])
	}
	cfg := m["configs"].(map[string]interface{})
	v := cfg["var"].([]interface{})[0].(map[string]interface{})
	if v["name"] != "内存" {
		t.Errorf("var.name is functional and must not be translated, got %v", v["name"])
	}
	if v["label"] != "Memory" {
		t.Errorf("var.label = %v", v["label"])
	}
	p := cfg["panels"].([]interface{})[0].(map[string]interface{})
	if p["name"] != "Memory" {
		t.Errorf("panels.name = %v", p["name"])
	}
	tgt := p["targets"].([]interface{})[0].(map[string]interface{})
	if tgt["legend"] != "QPS" || tgt["expr"] != "rate(x[1m])" {
		t.Errorf("targets = %v", tgt)
	}
	rn := p["transformations"].([]interface{})[0].(map[string]interface{})["options"].(map[string]interface{})["renameByName"].(map[string]interface{})
	if rn["col_a"] != "Renamed Column" {
		t.Errorf("renameByName value = %v", rn["col_a"])
	}
	nested := p["panels"].([]interface{})[0].(map[string]interface{})
	if nested["name"] != "Memory" {
		t.Errorf("nested panels.name = %v", nested["name"])
	}
	if nested["description"] != "未收录描述" {
		t.Errorf("missing dict entry should fall back, got %v", nested["description"])
	}
}

// valueMappings 的 options 形态（key 是被映射的原始值）与表格默认排序列
// custom.sortColumn（取值是 renameByName 之后的列名）都属展示字段，
// 必须与 result.text / renameByName 同步翻译，否则 en 下映射文案不一致、排序失效
func TestTranslateDashboardMapValueMappingsAndSortColumn(t *testing.T) {
	dict := ComponentDict{
		"永久":  "Permanent",
		"异常":  "Abnormal",
		"指标":  "Metric",
		"分组键": "GroupKey",
	}

	m := map[string]interface{}{
		"configs": map[string]interface{}{
			"panels": []interface{}{
				map[string]interface{}{
					"custom": map[string]interface{}{
						"sortColumn": "指标",
						"aggrFunc":   "分组键", // custom 下的非白名单字段不动
					},
					"options": map[string]interface{}{
						"valueMappings": []interface{}{
							map[string]interface{}{
								"options": map[string]interface{}{
									"-2": map[string]interface{}{"text": "永久", "color": "#fff"},
									"-1": map[string]interface{}{"text": "异常"},
								},
								"result": map[string]interface{}{"text": "永久"},
							},
						},
					},
					"transformations": []interface{}{
						map[string]interface{}{
							"options": map[string]interface{}{
								"renameByName": map[string]interface{}{"__name__": "指标"},
							},
						},
					},
				},
			},
		},
	}
	TranslateDashboardMap(m, dict)

	p := m["configs"].(map[string]interface{})["panels"].([]interface{})[0].(map[string]interface{})
	custom := p["custom"].(map[string]interface{})
	if custom["sortColumn"] != "Metric" {
		t.Errorf("custom.sortColumn = %v, want Metric (与 renameByName 同步)", custom["sortColumn"])
	}
	if custom["aggrFunc"] != "分组键" {
		t.Errorf("custom.aggrFunc is not a display field, got %v", custom["aggrFunc"])
	}
	vm := p["options"].(map[string]interface{})["valueMappings"].([]interface{})[0].(map[string]interface{})
	opts := vm["options"].(map[string]interface{})
	if opts["-2"].(map[string]interface{})["text"] != "Permanent" {
		t.Errorf("valueMappings.options text = %v", opts["-2"])
	}
	if opts["-1"].(map[string]interface{})["text"] != "Abnormal" {
		t.Errorf("valueMappings.options text = %v", opts["-1"])
	}
	if opts["-2"].(map[string]interface{})["color"] != "#fff" {
		t.Errorf("color must stay untouched, got %v", opts["-2"])
	}
	if vm["result"].(map[string]interface{})["text"] != "Permanent" {
		t.Errorf("valueMappings.result.text = %v", vm["result"])
	}
}

func TestTranslateDashboardMapConfigsAsString(t *testing.T) {
	dict := ComponentDict{"面板一": "Panel One"}
	m := map[string]interface{}{
		"name":    "board",
		"configs": `{"panels":[{"name":"面板一","id":1751791234000029}]}`,
	}
	TranslateDashboardMap(m, dict)

	configs := m["configs"].(string)
	if !strings.Contains(configs, "Panel One") {
		t.Errorf("configs string not translated: %s", configs)
	}
	// UseNumber 防大整数精度丢失
	if !strings.Contains(configs, "1751791234000029") {
		t.Errorf("big int precision lost: %s", configs)
	}
}

func newTestStore() *BuiltinPayloadInFileType {
	return NewBuiltinPayloadInFileType()
}

func alertPayload(componentID uint64, uuid int64, name string) *models.BuiltinPayload {
	content, _ := json.Marshal(map[string]interface{}{
		"name": name,
		"note": "",
		"uuid": uuid,
	})
	return &models.BuiltinPayload{
		ComponentID: componentID,
		Type:        "alert",
		Cate:        "linux_by_categraf",
		Name:        name,
		UUID:        uuid,
		Content:     string(content),
	}
}

func TestAddBuiltinPayloadRendersVariants(t *testing.T) {
	b := newTestStore()
	b.dicts[1] = map[string]ComponentDict{
		LangEnUS: {"磁盘使用率过高": "Disk usage too high"},
	}

	bp := alertPayload(1, 100, "磁盘使用率过高")
	b.AddBuiltinPayload(bp)

	zh := b.GetByUUID(100, LangSource)
	en := b.GetByUUID(100, LangEnUS)
	if zh == nil || en == nil {
		t.Fatal("payload missing in lang bucket")
	}
	if zh != bp {
		t.Error("source bucket must keep original pointer")
	}
	if en == bp {
		t.Error("en variant should be a rendered copy when dict hits")
	}
	if en.Name != "Disk usage too high" {
		t.Errorf("en name = %v", en.Name)
	}
	if !strings.Contains(en.Content, "Disk usage too high") {
		t.Errorf("en content not rendered: %s", en.Content)
	}
	if zh.Name != "磁盘使用率过高" || !strings.Contains(zh.Content, "磁盘使用率过高") {
		t.Errorf("source payload must stay untouched: %+v", zh)
	}
}

func TestAddBuiltinPayloadPassThroughWithoutDict(t *testing.T) {
	b := newTestStore()

	bp := alertPayload(2, 200, "磁盘使用率过高")
	b.AddBuiltinPayload(bp)

	if got := b.GetByUUID(200, LangEnUS); got != bp {
		t.Error("component without dict should reuse the same pointer in en bucket")
	}

	lst, err := b.GetBuiltinPayload("alert", "", "", 2, LangEnUS)
	if err != nil || len(lst) != 1 || lst[0] != bp {
		t.Errorf("GetBuiltinPayload en_US pass-through failed: %v %v", lst, err)
	}
}

func TestGetByUUIDFallback(t *testing.T) {
	b := newTestStore()
	bp := alertPayload(3, 300, "规则")
	// 只写源语言桶，模拟异常语言桶缺失
	b.addBuiltinPayloadForLang(LangSource, bp)

	if got := b.GetByUUID(300, LangEnUS); got != bp {
		t.Error("GetByUUID should fall back to source lang bucket")
	}
	if got := b.GetByUUID(999, LangEnUS); got != nil {
		t.Error("unknown uuid should return nil")
	}
}

func TestGetBuiltinPayloadSearchOnRenderedName(t *testing.T) {
	b := newTestStore()
	b.dicts[4] = map[string]ComponentDict{
		LangEnUS: {"磁盘使用率过高": "Disk usage too high"},
	}
	b.AddBuiltinPayload(alertPayload(4, 400, "磁盘使用率过高"))

	lst, err := b.GetBuiltinPayload("alert", "", "disk usage", 4, LangEnUS)
	if err != nil || len(lst) != 1 {
		t.Errorf("query on rendered en name should hit: %v %v", lst, err)
	}
	lst, err = b.GetBuiltinPayload("alert", "", "磁盘", 4, LangSource)
	if err != nil || len(lst) != 1 {
		t.Errorf("query on source name should hit: %v %v", lst, err)
	}
}

func TestRenderVariantTranslatesCate(t *testing.T) {
	b := newTestStore()
	b.dicts[5] = map[string]ComponentDict{
		LangEnUS: {"阿里云-RDS": "AliYun-RDS", "规则": "Rule"},
	}
	bp := alertPayload(5, 500, "规则")
	bp.Cate = "阿里云-RDS"
	b.AddBuiltinPayload(bp)

	cates, err := b.GetBuiltinPayloadCates("alert", 5, LangEnUS)
	if err != nil || len(cates) != 1 || cates[0] != "AliYun-RDS" {
		t.Errorf("en cates = %v %v", cates, err)
	}
	// 前端用翻译后的 cate 回查列表，必须能对上
	lst, err := b.GetBuiltinPayload("alert", "AliYun-RDS", "", 5, LangEnUS)
	if err != nil || len(lst) != 1 {
		t.Errorf("query by translated cate should hit: %v %v", lst, err)
	}
}

func TestPickReadmeFiles(t *testing.T) {
	// README.en_US.md 字典序在 README.md 之前，源必须仍是 README.md
	source, variants := PickReadmeFiles([]string{"README.en_US.md", "README.md", "arch.png"})
	if source != "README.md" {
		t.Errorf("source = %q", source)
	}
	if variants["en_US"] != "README.en_US.md" {
		t.Errorf("variants = %v", variants)
	}

	// 无精确 README.md 时退回首个非语言副本的 .md
	source, variants = PickReadmeFiles([]string{"README.en_US.md", "mysql.md"})
	if source != "mysql.md" || variants["en_US"] != "README.en_US.md" {
		t.Errorf("fallback source = %q, variants = %v", source, variants)
	}
}

func TestRenderVariantUnknownTypePassThrough(t *testing.T) {
	dict := ComponentDict{"x": "y"}
	bp := &models.BuiltinPayload{Type: "collect", Content: "# toml", Name: "x"}
	if got := renderVariant(bp, dict); got != bp {
		t.Error("unknown type must pass through the original pointer")
	}
}
