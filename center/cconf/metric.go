package cconf

import (
	"encoding/json"
	"path"

	"github.com/ccfos/nightingale/v6/pkg/i18nx"

	"github.com/toolkits/pkg/file"
)

// metricDesc , As load map happens before read map, there is no necessary to use concurrent map for metric desc store
//
// metrics.yaml 的顶层既可以是「指标名: 释义」（无语言维度，进 CommonDesc），
// 也可以是「语言: {指标名: 释义}」。语言键不限于 zh/en，写 ja、ja_JP 或任何
// 语言码都会被识别，无需改代码。
type MetricDescType struct {
	CommonDesc map[string]string            // 指标名 -> 释义，语言无关
	Langs      map[string]map[string]string // yaml 中的原始语言键 -> 指标名 -> 释义

	// byLang 是 Langs 按标准语言码建的查询索引（zh -> zh_CN、en -> en_US），
	// 让 metrics.yaml 里的短码与请求头里的标准码能对上
	byLang map[string]map[string]string
}

var MetricDesc MetricDescType

// UnmarshalYAML 按值的类型分流顶层键：标量值是语言无关的释义，映射值是某语言的
// 释义表。yaml.v2 把嵌套映射解成 map[interface{}]interface{}，需要逐项断言
func (m *MetricDescType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw map[string]interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}

	m.CommonDesc = make(map[string]string)
	m.Langs = make(map[string]map[string]string)

	for key, val := range raw {
		switch v := val.(type) {
		case string:
			m.CommonDesc[key] = v
		case map[interface{}]interface{}:
			dict := make(map[string]string, len(v))
			for mk, mv := range v {
				ks, kok := mk.(string)
				vs, vok := mv.(string)
				if kok && vok {
					dict[ks] = vs
				}
			}
			m.Langs[key] = dict
		}
	}

	m.buildLangIndex()
	return nil
}

// buildLangIndex 建标准语言码索引。同时写了 zh 和 zh_CN 这类同义键时以精确的
// 标准码为准，避免取值随 map 遍历顺序摇摆
func (m *MetricDescType) buildLangIndex() {
	m.byLang = make(map[string]map[string]string, len(m.Langs))
	for key, dict := range m.Langs {
		canonical := i18nx.NormalizeLang(key)
		if _, exists := m.byLang[canonical]; exists && key != canonical {
			continue
		}
		m.byLang[canonical] = dict
	}
}

// MarshalJSON 保持 GET /api/n9e/metrics/desc 的返回结构不变：语言表平铺在顶层，
// 语言无关的释义收在 common 下
func (m MetricDescType) MarshalJSON() ([]byte, error) {
	out := make(map[string]interface{}, len(m.Langs)+1)
	for lang, dict := range m.Langs {
		out[lang] = dict
	}
	out["common"] = m.CommonDesc
	return json.Marshal(out)
}

// metricDescLangChain 指标释义的语言回退链。
//
// 非中文语言不回退中文：宁可不展示释义，也不给读不懂的人显示中文——这与内置模板
// 不同，模板缺翻译时中文标题好过没有条目，而释义只是补充说明，缺了不影响使用
func metricDescLangChain(lang string) []string {
	switch normalized := i18nx.NormalizeLang(lang); normalized {
	case "", i18nx.LangZhCN:
		return []string{i18nx.LangZhCN}
	case i18nx.LangZhHK:
		return []string{i18nx.LangZhHK, i18nx.LangZhCN}
	case i18nx.LangEnUS:
		return []string{i18nx.LangEnUS}
	default:
		return []string{normalized, i18nx.LangEnUS}
	}
}

// GetMetricDesc , if metric is not registered, empty string will be returned
func GetMetricDesc(lang, metric string) string {
	for _, l := range metricDescLangChain(lang) {
		if m := MetricDesc.byLang[l]; m != nil {
			if desc, ok := m[metric]; ok {
				return desc
			}
		}
	}

	if MetricDesc.CommonDesc != nil {
		if desc, ok := MetricDesc.CommonDesc[metric]; ok {
			return desc
		}
	}

	return ""
}

func LoadMetricsYaml(configDir, metricsYamlFile string) error {
	fp := metricsYamlFile
	if fp == "" {
		fp = path.Join(configDir, "metrics.yaml")
	}
	if !file.IsExist(fp) {
		return nil
	}
	return file.ReadYaml(fp, &MetricDesc)
}
