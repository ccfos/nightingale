package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// newStoreWithPartialJa 造一个「日语只覆盖部分组件」的内存库：
// 组件 1 有 ja_JP 词条，组件 2 只有 en_US——这是新增语言时的常态
func newStoreWithPartialJa() *BuiltinPayloadInFileType {
	b := NewBuiltinPayloadInFileType()

	b.dicts[1] = map[string]ComponentDict{
		LangEnUS: {"磁盘使用率过高": "Disk usage too high"},
		"ja_JP":  {"磁盘使用率过高": "ディスク使用率が高すぎます"},
	}
	b.dicts[2] = map[string]ComponentDict{
		LangEnUS: {"内存使用率过高": "Memory usage too high"},
	}

	b.AddBuiltinPayload(&models.BuiltinPayload{
		ComponentID: 1, Type: "alert", Cate: "disk",
		Name: "磁盘使用率过高", UUID: 1001,
		Content: `{"name":"磁盘使用率过高"}`,
	})
	b.AddBuiltinPayload(&models.BuiltinPayload{
		ComponentID: 2, Type: "alert", Cate: "mem",
		Name: "内存使用率过高", UUID: 1002,
		Content: `{"name":"内存使用率过高"}`,
	})

	return b
}

// TestGetBuiltinPayloadPartialLangFallback 覆盖按桶回退会踩的回归：
// ja_JP 桶一旦非空，未翻译的组件会查到空 map，若只判空桶就会返回空列表——
// 日语用户看到的不是英文模板，而是「这个组件没有任何模板」
func TestGetBuiltinPayloadPartialLangFallback(t *testing.T) {
	b := newStoreWithPartialJa()

	translated, err := b.GetBuiltinPayload("alert", "", "", 1, "ja_JP")
	if err != nil {
		t.Fatalf("get component 1 fail: %v", err)
	}
	if len(translated) != 1 || translated[0].Name != "ディスク使用率が高すぎます" {
		t.Errorf("component with ja dict should render Japanese, got %+v", translated)
	}

	fellBack, err := b.GetBuiltinPayload("alert", "", "", 2, "ja_JP")
	if err != nil {
		t.Fatalf("get component 2 fail: %v", err)
	}
	if len(fellBack) != 1 {
		t.Fatalf("component without ja dict must fall back to en_US, got %d payloads", len(fellBack))
	}
	if fellBack[0].Name != "Memory usage too high" {
		t.Errorf("fallback should be English, got %q", fellBack[0].Name)
	}
}

func TestGetBuiltinPayloadCatesPartialLangFallback(t *testing.T) {
	b := newStoreWithPartialJa()

	cates, err := b.GetBuiltinPayloadCates("alert", 2, "ja_JP")
	if err != nil {
		t.Fatalf("get cates fail: %v", err)
	}
	if len(cates) != 1 || cates[0] != "mem" {
		t.Errorf("cates should fall back to en_US bucket, got %v", cates)
	}
}

func TestGetByUUIDLangFallback(t *testing.T) {
	b := newStoreWithPartialJa()

	if bp := b.GetByUUID(1001, "ja_JP"); bp == nil || bp.Name != "ディスク使用率が高すぎます" {
		t.Errorf("uuid 1001 should resolve to Japanese variant, got %+v", bp)
	}
	if bp := b.GetByUUID(1002, "ja_JP"); bp == nil || bp.Name != "Memory usage too high" {
		t.Errorf("uuid 1002 should fall back to English variant, got %+v", bp)
	}
	// 未知语言同样走 <lang> → en_US → zh_CN
	if bp := b.GetByUUID(1002, "fr_FR"); bp == nil || bp.Name != "Memory usage too high" {
		t.Errorf("unknown lang should fall back to English, got %+v", bp)
	}
}

func TestReadmeLangFallback(t *testing.T) {
	b := NewBuiltinPayloadInFileType()
	b.Readmes = map[string]map[string]string{
		LangEnUS: {"MySQL": "english readme", "Redis": "english readme"},
		"ja_JP":  {"MySQL": "日本語 readme"},
	}

	if got := b.Readme("ja_JP", "MySQL"); got != "日本語 readme" {
		t.Errorf("MySQL ja readme = %q, want Japanese copy", got)
	}
	if got := b.Readme("ja_JP", "Redis"); got != "english readme" {
		t.Errorf("Redis ja readme = %q, want English fallback", got)
	}
	// 没有任何语言副本时返回空串，调用方保留 DB 中的源语言 README
	if got := b.Readme("ja_JP", "Kafka"); got != "" {
		t.Errorf("missing readme should return empty, got %q", got)
	}
	// 繁体不经英文回退，直接用 DB 里的简体原文
	if got := b.Readme(LangZhHK, "Redis"); got != "" {
		t.Errorf("zh_HK must not fall back to English, got %q", got)
	}
}

// TestLoadComponentDictsNormalizesLangKey 词条文件名可以写短码也可以写标准码，
// 两者必须建到同一个桶：桶名不归一化时 ja.json 建出的桶永远等不到查询，
// 且没有任何报错——翻译者会以为词条没生效
func TestLoadComponentDictsNormalizesLangKey(t *testing.T) {
	for _, fileName := range []string{"ja.json", "ja_JP.json", "ja-JP.json"} {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "i18n"), 0o755); err != nil {
			t.Fatalf("mkdir fail: %v", err)
		}
		content := `{"磁盘使用率过高":"ディスク使用率が高すぎます"}`
		if err := os.WriteFile(filepath.Join(dir, "i18n", fileName), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s fail: %v", fileName, err)
		}

		dicts := LoadComponentDicts(dir)
		if _, ok := dicts["ja_JP"]; !ok {
			t.Errorf("%s should build the ja_JP bucket, got buckets %v", fileName, keysOf(dicts))
		}
	}
}

func keysOf(m map[string]ComponentDict) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestResolveBucketLang 没有词条桶的语言必须收敛到回退链末端，
// 否则按语言缓存的调用方 key 空间会跟着请求头膨胀
func TestResolveBucketLang(t *testing.T) {
	b := newStoreWithPartialJa()

	if got := b.ResolveBucketLang("ja_JP"); got != "ja_JP" {
		t.Errorf("ja_JP has a bucket, got %q", got)
	}
	// 无桶语言（含随手编造的请求头）统一收敛，不新增 key
	for _, lang := range []string{"fr_FR", "de_DE", "xx_YY"} {
		if got := b.ResolveBucketLang(lang); got != LangEnUS {
			t.Errorf("ResolveBucketLang(%q) = %q, want %q", lang, got, LangEnUS)
		}
	}
	if got := b.ResolveBucketLang(LangZhHK); got != LangSource {
		t.Errorf("zh_HK without bucket should collapse to zh_CN, got %q", got)
	}
}
