package cconf

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// 存量 metrics.yaml 的形态：顶层 zh/en 两张表，另有语言无关的散条目
const legacyMetricsYaml = `
some_common_metric: common desc
zh:
  cpu_usage_idle: CPU空闲率
  disk_free: 硬盘分区剩余量
en:
  cpu_usage_idle: CPU idle percentage
  disk_free: Disk partition free space
  mem_used: Memory used
`

// 新增语言只需在 yaml 里加一段，不用改代码
const multiLangMetricsYaml = `
zh:
  cpu_usage_idle: CPU空闲率
en:
  cpu_usage_idle: CPU idle percentage
  disk_free: Disk partition free space
ja:
  cpu_usage_idle: CPU アイドル率
`

func loadYaml(t *testing.T, content string) {
	t.Helper()
	MetricDesc = MetricDescType{}
	if err := yaml.Unmarshal([]byte(content), &MetricDesc); err != nil {
		t.Fatalf("unmarshal metrics yaml fail: %v", err)
	}
}

func TestLegacyMetricsYamlStillParses(t *testing.T) {
	loadYaml(t, legacyMetricsYaml)

	if got := GetMetricDesc("zh_CN", "cpu_usage_idle"); got != "CPU空闲率" {
		t.Errorf("zh_CN desc = %q", got)
	}
	if got := GetMetricDesc("en", "cpu_usage_idle"); got != "CPU idle percentage" {
		t.Errorf("en desc = %q", got)
	}
	// 空语言按中文处理，与页面接口的缺省一致
	if got := GetMetricDesc("", "cpu_usage_idle"); got != "CPU空闲率" {
		t.Errorf("empty lang desc = %q, want Chinese", got)
	}
	// 顶层散条目仍作为语言无关兜底
	if got := GetMetricDesc("en", "some_common_metric"); got != "common desc" {
		t.Errorf("common desc = %q", got)
	}
	// 该语言没有的指标不跨语言乱回退，返回空串
	if got := GetMetricDesc("zh_CN", "mem_used"); got != "" {
		t.Errorf("zh has no mem_used, want empty, got %q", got)
	}
}

// TestNonChineseLangFallsBackToEnglish 覆盖改造前的 bug：日语等未列举的语言
// 走 default 分支拿到中文表，日语用户看到的是中文释义
func TestNonChineseLangFallsBackToEnglish(t *testing.T) {
	loadYaml(t, legacyMetricsYaml)

	if got := GetMetricDesc("ja_JP", "cpu_usage_idle"); got != "CPU idle percentage" {
		t.Errorf("ja_JP desc = %q, want English fallback (not Chinese)", got)
	}
	if got := GetMetricDesc("ru_RU", "disk_free"); got != "Disk partition free space" {
		t.Errorf("ru_RU desc = %q, want English fallback", got)
	}
}

func TestMultiLangMetricsYaml(t *testing.T) {
	loadYaml(t, multiLangMetricsYaml)

	// yaml 里的短码 ja 与请求头里的 ja_JP 是同一门语言
	if got := GetMetricDesc("ja_JP", "cpu_usage_idle"); got != "CPU アイドル率" {
		t.Errorf("ja_JP desc = %q, want Japanese", got)
	}
	// 日语没收录的指标退到英文
	if got := GetMetricDesc("ja_JP", "disk_free"); got != "Disk partition free space" {
		t.Errorf("ja_JP fallback = %q, want English", got)
	}
	// 繁体退简体
	if got := GetMetricDesc("zh_HK", "cpu_usage_idle"); got != "CPU空闲率" {
		t.Errorf("zh_HK desc = %q, want Simplified Chinese fallback", got)
	}
}

// TestMarshalJSONKeepsShape 保证 GET /api/n9e/metrics/desc 的返回结构不变
func TestMarshalJSONKeepsShape(t *testing.T) {
	loadYaml(t, legacyMetricsYaml)

	bs, err := json.Marshal(MetricDesc)
	if err != nil {
		t.Fatalf("marshal fail: %v", err)
	}

	var out map[string]map[string]string
	if err := json.Unmarshal(bs, &out); err != nil {
		t.Fatalf("unmarshal fail: %v", err)
	}
	if out["zh"]["cpu_usage_idle"] != "CPU空闲率" {
		t.Errorf("zh bucket missing from JSON: %s", bs)
	}
	if out["en"]["cpu_usage_idle"] != "CPU idle percentage" {
		t.Errorf("en bucket missing from JSON: %s", bs)
	}
	if out["common"]["some_common_metric"] != "common desc" {
		t.Errorf("common bucket missing from JSON: %s", bs)
	}
}

// TestCanonicalKeyWinsOverAlias 同时写了 zh 和 zh_CN 时取值不应随 map 遍历顺序摇摆
func TestCanonicalKeyWinsOverAlias(t *testing.T) {
	const dup = `
zh:
  cpu_usage_idle: 短码
zh_CN:
  cpu_usage_idle: 标准码
`
	for i := 0; i < 20; i++ {
		loadYaml(t, dup)
		if got := GetMetricDesc("zh_CN", "cpu_usage_idle"); got != "标准码" {
			t.Fatalf("round %d: got %q, want the canonical key to win", i, got)
		}
	}
}

// TestBundledMetricsYamlLangCoverage 钉住随包发布的 etc/metrics.yaml：
// 日语必须与英文覆盖同一批指标。新增英文条目却漏掉日语时，日语用户会静默退回英文，
// 这里让它在 CI 暴露而不是等用户发现
func TestBundledMetricsYamlLangCoverage(t *testing.T) {
	MetricDesc = MetricDescType{}
	if err := LoadMetricsYaml("../../etc", ""); err != nil {
		t.Fatalf("load bundled metrics.yaml fail: %v", err)
	}

	zh, en, ja := MetricDesc.Langs["zh"], MetricDesc.Langs["en"], MetricDesc.Langs["ja"]
	if len(en) == 0 || len(ja) == 0 {
		t.Fatalf("bundled metrics.yaml missing en(%d) or ja(%d) section", len(en), len(ja))
	}

	// 中文是这份文件的源语言，英文必须覆盖它的全部条目：只写中文的指标，
	// 非中文用户看到的是空白说明
	var noEn []string
	for metric := range zh {
		if _, ok := en[metric]; !ok {
			noEn = append(noEn, metric)
		}
	}
	sort.Strings(noEn)
	if len(noEn) > 0 {
		t.Errorf("%d metrics have zh desc but no en: %v", len(noEn), noEn)
	}

	var missing, extra []string
	for metric := range en {
		if _, ok := ja[metric]; !ok {
			missing = append(missing, metric)
		}
	}
	for metric := range ja {
		if _, ok := en[metric]; !ok {
			extra = append(extra, metric)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("%d metrics have en desc but no ja: %v", len(missing), missing)
	}
	if len(extra) > 0 {
		t.Errorf("%d metrics have ja desc but no en: %v", len(extra), extra)
	}

	// 漏译的中文会原样留在日语段里。日语本身也用汉字，无法按字符集判定，
	// 只能挑几个简体中文特有的词做抽查
	for metric, desc := range ja {
		for _, bad := range []string{"个数", "总数量", "已用内存数", "网卡收包", "剩余量", "空闲率"} {
			if strings.Contains(desc, bad) {
				t.Errorf("ja desc of %s looks like untranslated Chinese: %q", metric, desc)
			}
		}
	}
}
