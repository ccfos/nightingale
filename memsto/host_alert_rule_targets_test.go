package memsto

import (
	"reflect"
	"testing"
)

// TestTargetsOfAlertRuleCacheGetByEngine 覆盖按引擎取快照：只返回该引擎的数据、
// 未知引擎返回 has=false，且返回值是拷贝，改写它不会污染缓存。
func TestTargetsOfAlertRuleCacheGetByEngine(t *testing.T) {
	tc := &TargetsOfAlertRuleCacheType{targets: make(map[string]map[int64][]string)}
	tc.Set(map[string]map[int64][]string{
		"edge-a": {1: {"host-1", "host-2"}, 2: {"host-3"}},
		"edge-b": {1: {"host-9"}},
	}, 0, 0)

	got, has := tc.GetByEngine("edge-a")
	if !has {
		t.Fatal("GetByEngine(edge-a) has = false, want true")
	}
	want := map[int64][]string{1: {"host-1", "host-2"}, 2: {"host-3"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetByEngine(edge-a) = %v, want %v", got, want)
	}

	if _, has := tc.GetByEngine("edge-unknown"); has {
		t.Fatal("GetByEngine(edge-unknown) has = true, want false")
	}

	// 改写返回值，缓存内容必须保持不变
	got[1][0] = "tampered"
	got[3] = []string{"injected"}
	again, _ := tc.GetByEngine("edge-a")
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("cache mutated through GetByEngine result: %v, want %v", again, want)
	}
	if lst, _ := tc.Get("edge-a", 1); lst[0] != "host-1" {
		t.Fatalf("Get(edge-a, 1)[0] = %q, want host-1", lst[0])
	}
}
