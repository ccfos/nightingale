package models

import "testing"

func TestMatchCate(t *testing.T) {
	cases := []struct {
		name      string
		subCate   string
		eventCate string
		want      bool
	}{
		{"sub cate empty matches all", "", "elasticsearch", true},
		{"event cate empty matches all", "host", "", true},
		{"host exact match", "host", "host", true},
		{"host mismatch", "host", "prometheus", false},
		{"exact match", "elasticsearch", "elasticsearch", true},
		{"mismatch", "elasticsearch", "tdengine", false},
		{"mismatch against host", "elasticsearch", "host", false},
		// 存量订阅的 prometheus 是历史表单默认值，视为不过滤，但 host 事件除外
		{"legacy prometheus matches all", "prometheus", "elasticsearch", true},
		{"legacy prometheus does not match host", "prometheus", "host", false},
		{"legacy prometheus matches itself", "prometheus", "prometheus", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &AlertSubscribe{Cate: c.subCate}
			if got := s.MatchCate(c.eventCate); got != c.want {
				t.Errorf("MatchCate(sub=%q, event=%q) = %v, want %v", c.subCate, c.eventCate, got, c.want)
			}
		})
	}
}
