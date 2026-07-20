package integration

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

// 内置模板 i18n：模板文件本身是源语言（中文），其他语言的词条放在
// integrations/<Component>/i18n/<lang>.json（flat map，中文原文作 key），
// 加载时按显示字段白名单查表渲染出对应语言的变体，同一 uuid 多语言。
// 词条缺失时按单字段粒度回退源语言原文。
const (
	LangSource = "zh_CN"
	LangEnUS   = "en_US"
)

// ComponentDict 单个组件单语言的词条表：源语言原文 -> 译文
type ComponentDict map[string]string

// NormalizeLang 归一化 X-Language 请求头：空值与 zh 前缀视为源语言（zh_CN），
// 其余（含 en 前缀和未支持语言）归入 en_US——en_US 桶对全部内置内容完整存在
// （无词条的组件为 pass-through），因此不需要列表级回退。
// 大小写不敏感：前端只发 zh_CN，但第三方直连 API 可能传 ZH-CN
func NormalizeLang(lang string) string {
	if lang == "" || strings.HasPrefix(strings.ToLower(lang), "zh") {
		return LangSource
	}
	return LangEnUS
}

// LoadComponentDicts 读取组件目录下 i18n/<lang>.json 词条文件
func LoadComponentDicts(componentDir string) map[string]ComponentDict {
	dicts := make(map[string]ComponentDict)

	files, err := file.FilesUnder(componentDir + "/i18n")
	if err != nil {
		// i18n 目录不存在是常态（组件本身即英文或词条未提供）
		return dicts
	}

	for _, f := range files {
		if !strings.HasSuffix(f, ".json") {
			continue
		}
		lang := strings.TrimSuffix(f, ".json")

		bs, err := file.ReadBytes(componentDir + "/i18n/" + f)
		if err != nil {
			logger.Warningf("read builtin component i18n file fail %s/%s %v", componentDir, f, err)
			continue
		}

		dict := make(ComponentDict)
		if err := json.Unmarshal(bs, &dict); err != nil {
			logger.Warningf("parse builtin component i18n file fail %s/%s %v", componentDir, f, err)
			continue
		}
		dicts[lang] = dict
	}

	return dicts
}

// readmeVariantRe 匹配语言副本文件名，如 README.en_US.md
var readmeVariantRe = regexp.MustCompile(`^README\.([a-zA-Z]{2}(?:_[a-zA-Z]{2})?)\.md$`)

// PickReadmeFiles 从 markdown 目录文件列表中选出源 README 与各语言副本。
// 源优先取精确的 README.md；不存在时退回首个非语言副本的 .md 文件
// （不能简单取第一个 .md：README.en_US.md 字典序排在 README.md 之前）
func PickReadmeFiles(files []string) (source string, variants map[string]string) {
	variants = make(map[string]string)
	for _, f := range files {
		if m := readmeVariantRe.FindStringSubmatch(f); m != nil {
			variants[m[1]] = f
			continue
		}
		if f == "README.md" {
			source = f
			continue
		}
		if source == "" && strings.HasSuffix(strings.ToLower(f), ".md") {
			source = f
		}
	}
	return source, variants
}

// Translate 查词条表，miss 时返回原文——单字段粒度回退的实现点
func Translate(dict ComponentDict, s string) string {
	if t, ok := dict[s]; ok && t != "" {
		return t
	}
	return s
}

// displayFieldPathSuffixes 仪表盘 configs 内展示字段的路径后缀白名单
// （路径为 key 链，数组下标不计入）。只有路径命中白名单且词条表存在原文
// 两个条件同时满足才替换，避免误伤功能字段（如 var[].name 是 promql 里
// $name 引用的变量标识符，与展示字段 var[].label 只差一个 key）。
var displayFieldPathSuffixes = []string{
	"panels.name",        // 面板标题（含嵌套 panels.panels.name）
	"panels.description", // 面板描述
	"targets.legend",     // 曲线图例
	"var.label",          // 变量展示名（var.name 是功能标识，不在白名单）
	"valueMappings.result.text",
	"matcher.value",       // overrides 按图例/字段名匹配，需与被翻译的 legend 保持一致
	"custom.sortColumn",   // 表格默认排序列，取值是 renameByName 后的列名，需同步翻译
	"links.title",
	"custom.detailName",
	"standardOptions.util",
	"standardOptions.displayName",
	"var.definition", // 自定义变量的 "标签 : 值" 定义串，整串作 key 翻译（值部分随词条保留）
}

// displayFieldPathContains renameByName 类结构的 value 全部是展示用重命名，
// 其 key 是数据列名（动态），无法用固定后缀表达
var displayFieldPathContains = []string{
	"renameByName",
}

// IsDisplayFieldPath 判定仪表盘 configs 内的字段路径是否属于展示字段白名单
// （路径为 key 链，数组下标不计入，根段是 "configs"）。导出给 CI 工具复用，
// 保证门禁分类口径与运行时翻译口径同源
func IsDisplayFieldPath(path []string) bool {
	return pathMatchesWhitelist(path)
}

func pathMatchesWhitelist(path []string) bool {
	joined := strings.Join(path, ".")
	for _, suffix := range displayFieldPathSuffixes {
		if joined == suffix || strings.HasSuffix(joined, "."+suffix) {
			return true
		}
	}
	for _, elem := range displayFieldPathContains {
		for _, p := range path {
			if p == elem {
				return true
			}
		}
	}
	// valueMappings 的 options 形态：options 下的 key 是被映射的原始值（"-1"/"-2" 等
	// 动态标识），固定后缀表达不了，凡 valueMappings 子树里的 text 都是展示文案
	if len(path) > 0 && path[len(path)-1] == "text" {
		for _, p := range path {
			if p == "valueMappings" {
				return true
			}
		}
	}
	return false
}

// walkConfigs 递归遍历仪表盘 configs，对白名单路径上的字符串值应用 visit
// （翻译与词条提取共用同一份遍历，保证 CI 判定与运行时行为一致）
func walkConfigs(node interface{}, path []string, visit func(string) string) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		for k, child := range v {
			v[k] = walkConfigs(child, append(path, k), visit)
		}
		return v
	case []interface{}:
		for i, child := range v {
			v[i] = walkConfigs(child, path, visit)
		}
		return v
	case string:
		if pathMatchesWhitelist(path) {
			return visit(v)
		}
		return v
	default:
		return v
	}
}

// unmarshalUseNumber 解析 content JSON。必须用 UseNumber：内置模板的 uuid
// 是 UnixMicro 级 int64，走 float64 会丢精度
func unmarshalUseNumber(content string) (map[string]interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(content)))
	dec.UseNumber()
	m := make(map[string]interface{})
	err := dec.Decode(&m)
	return m, err
}

// VisitAlertDisplayFields 遍历告警规则对象的展示字段并应用 visit：name、note、
// annotations 的 key 与 value（key 在事件详情页按原样展示，无代码按名字引用，
// 因此同属展示字段）。promql、append_tags 等功能字段一律不遍历
func VisitAlertDisplayFields(m map[string]interface{}, visit func(string) string) {
	if s, ok := m["name"].(string); ok {
		m["name"] = visit(s)
	}
	if s, ok := m["note"].(string); ok {
		m["note"] = visit(s)
	}
	if ann, ok := m["annotations"].(map[string]interface{}); ok {
		translated := make(map[string]interface{}, len(ann))
		for k, v := range ann {
			if s, ok := v.(string); ok {
				v = visit(s)
			}
			nk := visit(k)
			if _, dup := translated[nk]; dup && nk != k {
				// 两个 key 译成同一个时保留原 key，宁可漏译不丢条目
				nk = k
			}
			translated[nk] = v
		}
		m["annotations"] = translated
	}
}

// VisitDashboardDisplayFields 遍历仪表盘对象的展示字段并应用 visit：根级
// name/note/tags + configs 内的白名单字段。configs 兼容对象和 JSON 字符串两种历史形态
func VisitDashboardDisplayFields(m map[string]interface{}, visit func(string) string) {
	if s, ok := m["name"].(string); ok {
		m["name"] = visit(s)
	}
	if s, ok := m["note"].(string); ok {
		m["note"] = visit(s)
	}
	// 仪表盘 tags 是列表页展示/检索标签（区别于告警的 append_tags 功能字段）
	if s, ok := m["tags"].(string); ok {
		m["tags"] = visit(s)
	}

	switch configs := m["configs"].(type) {
	case map[string]interface{}, []interface{}:
		m["configs"] = walkConfigs(configs, []string{"configs"}, visit)
	case string:
		dec := json.NewDecoder(bytes.NewReader([]byte(configs)))
		dec.UseNumber()
		var parsed interface{}
		if err := dec.Decode(&parsed); err != nil {
			return
		}
		parsed = walkConfigs(parsed, []string{"configs"}, visit)
		bs, err := json.Marshal(parsed)
		if err != nil {
			return
		}
		m["configs"] = string(bs)
	}
}

// TranslateAlertMap 就地翻译告警规则对象的展示字段
func TranslateAlertMap(m map[string]interface{}, dict ComponentDict) {
	VisitAlertDisplayFields(m, func(s string) string { return Translate(dict, s) })
}

// TranslateDashboardMap 就地翻译仪表盘对象的展示字段
func TranslateDashboardMap(m map[string]interface{}, dict ComponentDict) {
	VisitDashboardDisplayFields(m, func(s string) string { return Translate(dict, s) })
}

// renderVariant 渲染一条内置 payload 的目标语言变体。词条为空或类型未接入
// 翻译时返回原指针（pass-through）；否则深拷贝，行级 Name/Cate 与 content
// 内的展示字段同步替换（content 必须同步：导入落库取的是 content）
func renderVariant(bp *models.BuiltinPayload, dict ComponentDict) *models.BuiltinPayload {
	if len(dict) == 0 {
		return bp
	}

	switch bp.Type {
	case "alert", "dashboard":
	default:
		// collect/firemap（n9e-plus 灌入）等类型暂未接入翻译
		return bp
	}

	m, err := unmarshalUseNumber(bp.Content)
	if err != nil {
		logger.Warningf("parse builtin payload content fail, skip i18n render, uuid: %d, err: %v", bp.UUID, err)
		return bp
	}

	switch bp.Type {
	case "alert":
		TranslateAlertMap(m, dict)
	case "dashboard":
		TranslateDashboardMap(m, dict)
	}

	bs, err := json.Marshal(m)
	if err != nil {
		logger.Warningf("marshal builtin payload content fail, skip i18n render, uuid: %d, err: %v", bp.UUID, err)
		return bp
	}

	variant := *bp
	variant.Content = string(bs)
	variant.Name = Translate(dict, bp.Name)
	// 告警模板的 cate 来自文件名，存在中文文件名（如 阿里云-RDS），是界面可见的分类项
	variant.Cate = Translate(dict, bp.Cate)
	variant.Note = Translate(dict, bp.Note)
	if bp.Type == "dashboard" {
		// 仪表盘行级 Tags 与 content 内 tags 同源，同步翻译；告警的 Tags 是 append_tags，不译
		variant.Tags = Translate(dict, bp.Tags)
	}
	return &variant
}
