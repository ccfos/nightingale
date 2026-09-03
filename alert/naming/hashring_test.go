package naming

import (
	"testing"
)

// TestDatasourceHashRingMembers 锁定 Members 的两个不变量：
//  1. 返回结果按字典序稳定排序——consistent.Members() 遍历 map、顺序不定，
//     上层用它做签名/变更检测，顺序抖动会误判"环成员变了"而频繁失效变更门。
//  2. ring 不存在时返回 nil，不 panic。
func TestDatasourceHashRingMembers(t *testing.T) {
	const key = "test-engine"
	DatasourceHashRing.Set(key, NewConsistentHashRing(NodeReplicas, []string{"10.0.0.3:9000", "10.0.0.1:9000", "10.0.0.2:9000"}))
	defer DatasourceHashRing.Del(key)

	got := DatasourceHashRing.Members(key)
	want := []string{"10.0.0.1:9000", "10.0.0.2:9000", "10.0.0.3:9000"}
	if len(got) != len(want) {
		t.Fatalf("members len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("members not sorted: got %v want %v", got, want)
		}
	}

	if m := DatasourceHashRing.Members("no-such-engine"); m != nil {
		t.Fatalf("members of absent ring should be nil, got %v", m)
	}
}
