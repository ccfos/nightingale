package chat

import "testing"

// pickBusiGroup 是纯函数，覆盖名称/ID 两种写法及各类边界。
func TestPickBusiGroup(t *testing.T) {
	groups := []busiGroupRef{
		{256, "aadddd"},
		{1, "Default Busi Group"},
		{251, "A研发自测"},
		{62, "aws"},
		{300, "aws-prod"},
		{5, "a"}, // 1 字符名，应被忽略
	}

	cases := []struct {
		name   string
		input  string
		wantID int64
		wantOK bool
	}{
		{"name exact quoted", `在 "aadddd" 业务组创建一个 Linux 仪表盘`, 256, true},
		{"name chinese", "在 A研发自测 业务组建告警规则", 251, true},
		{"id after keyword", "在业务组 256 创建仪表盘", 256, true},
		{"id group_id form", "group_id=251 建个面板", 251, true},
		{"id before keyword", "在 256 号业务组创建", 256, true},
		{"nested name picks longest", "在 aws-prod 业务组创建", 300, true},
		{"unknown id defers", "在业务组 999 创建", 0, false},
		{"stray number not adjacent", "CPU>80% 持续1分钟的告警", 0, false},
		{"no group mentioned", "创建一个仪表盘", 0, false},
		{"conflicting refs defer", "在 aadddd 业务组 1 创建", 0, false},
		{"one-rune name ignored", "建到 a 里", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := pickBusiGroup(groups, c.input)
			if id != c.wantID || ok != c.wantOK {
				t.Fatalf("pickBusiGroup(%q) = (%d,%v), want (%d,%v)", c.input, id, ok, c.wantID, c.wantOK)
			}
		})
	}
}

func TestRequiresContext(t *testing.T) {
	if !requiresContext([]string{"busi_group_id", "datasource_id"}, "busi_group_id") {
		t.Fatal("should require busi_group_id")
	}
	if requiresContext([]string{"team_ids"}, "busi_group_id") {
		t.Fatal("should not require busi_group_id")
	}
}
