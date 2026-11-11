package integration

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// newMetricCache 造一份进程级缓存，元素带 zh_CN/en_US 两种翻译，
// 且 Name/Note 的原始值与任何一种翻译都不相同，这样一旦缓存被就地改写就能被观察到。
func newMetricCache() *BuiltinPayloadInFileType {
	return &BuiltinPayloadInFileType{
		BuiltinMetrics: map[string]*models.BuiltinMetric{
			"cpu_usage_idle": {
				Collector:  "categraf",
				Typ:        "Node",
				Name:       "raw_name",
				Note:       "raw_note",
				Expression: "cpu_usage_idle",
				Translation: []models.Translation{
					{Lang: "zh_CN", Name: "CPU空闲率", Note: "CPU空闲百分比"},
					{Lang: "en_US", Name: "cpu idle", Note: "cpu idle percent"},
				},
			},
		},
	}
}

// 一次查询不应改写进程级缓存里的内容。缓存是所有请求共享的，
// 就地写入会让后续请求看到上一次请求的语言。
func TestBuiltinMetricGets_DoesNotMutateCache(t *testing.T) {
	b := newMetricCache()
	cached := b.BuiltinMetrics["cpu_usage_idle"]

	if _, _, err := b.BuiltinMetricGets(nil, "zh_CN", "", "", "", "", 20, 0); err != nil {
		t.Fatalf("BuiltinMetricGets: %v", err)
	}

	if cached.Name != "raw_name" || cached.Note != "raw_note" {
		t.Errorf("cache mutated by query: Name=%q Note=%q, want raw_name/raw_note",
			cached.Name, cached.Note)
	}
}

// query 过滤跑在翻译之前，读的是缓存里的 Name/Note。若翻译被写回缓存，
// 同一个 query 在第一次和后续请求会命中不同的行，total 随之漂移 —— 即 issue #3212
// 里“翻到第二页总数就变了”的现象。
func TestBuiltinMetricGets_QueryTotalIsStable(t *testing.T) {
	b := newMetricCache()

	_, firstTotal, err := b.BuiltinMetricGets(nil, "zh_CN", "", "", "raw_name", "", 20, 0)
	if err != nil {
		t.Fatalf("first query: %v", err)
	}

	_, secondTotal, err := b.BuiltinMetricGets(nil, "zh_CN", "", "", "raw_name", "", 20, 0)
	if err != nil {
		t.Fatalf("second query: %v", err)
	}

	if firstTotal != secondTotal {
		t.Errorf("total drifted between identical queries: %d then %d", firstTotal, secondTotal)
	}
}
