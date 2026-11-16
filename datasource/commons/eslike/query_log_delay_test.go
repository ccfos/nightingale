package eslike

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
)

// captureLogRange 执行 QueryLog 并返回发给 ES 的时间范围（epoch 毫秒）。
func captureLogRange(t *testing.T, ctx context.Context, q *Query) (gte, lt int64) {
	t.Helper()
	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		var body interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		r, ok := findRange(body, q.DateField)
		if !ok {
			t.Fatalf("range on %s not found in %s", q.DateField, raw)
		}
		gte, lt = int64(r["gte"].(float64)), int64(r["lt"].(float64))
		return &elastic.SearchResult{Hits: &elastic.SearchHits{}}, nil
	}
	if _, _, err := QueryLog(ctx, q, 10, "7.10+", 5, search); err != nil {
		t.Fatal(err)
	}
	return gte, lt
}

// findRange 在查询体里递归找到 field 上的 range 条件，不依赖 bool 子句的具体结构。
func findRange(node interface{}, field string) (map[string]interface{}, bool) {
	switch v := node.(type) {
	case map[string]interface{}:
		if rng, ok := v["range"].(map[string]interface{}); ok {
			if r, ok := rng[field].(map[string]interface{}); ok {
				return r, true
			}
		}
		for _, child := range v {
			if r, ok := findRange(child, field); ok {
				return r, true
			}
		}
	case []interface{}:
		for _, child := range v {
			if r, ok := findRange(child, field); ok {
				return r, true
			}
		}
	}
	return nil, false
}

func TestQueryLogAppliesDelayToRelativeWindow(t *testing.T) {
	ctx := context.WithValue(context.Background(), "delay", int64(30))
	before := time.Now().Unix()
	gte, lt := captureLogRange(t, ctx, &Query{Index: "logs-*", DateField: "ts", Interval: 90})
	after := time.Now().Unix()

	if lt < (before-30)*1000 || lt > (after-30)*1000 {
		t.Fatalf("lt=%d, want now-30s in [%d, %d]", lt, (before-30)*1000, (after-30)*1000)
	}
	if lt-gte != 90*1000 {
		t.Fatalf("window=%dms, want 90000ms", lt-gte)
	}
}

func TestQueryLogKeepsExplicitWindowUnderDelay(t *testing.T) {
	ctx := context.WithValue(context.Background(), "delay", int64(30))
	gte, lt := captureLogRange(t, ctx, &Query{Index: "logs-*", DateField: "ts", Start: 1700000000, End: 1700000060})

	if gte != 1700000000*1000 || lt != 1700000060*1000 {
		t.Fatalf("got [%d, %d), want explicit window untouched", gte, lt)
	}
}
