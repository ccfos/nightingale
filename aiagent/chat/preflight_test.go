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

// pickDatasource 是纯函数，覆盖名称/ID 两种写法及各类边界（含真实日志里的两条复现）。
func TestPickDatasource(t *testing.T) {
	dsList := []datasourceRef{
		{694, "ali_prom"},
		{1, "demo-vm"},
		{42, "prod-mysql"},
		{43, "prod"}, // 与 prod-mysql 互为包含，按最长取
	}

	cases := []struct {
		name   string
		input  string
		wantID int64
		wantOK bool
	}{
		{"name from text 数据源：demo-vm", "请创建一条主机CPU告警规则：- 业务组：Default - 数据源：demo-vm", 1, true},
		{"id from text datasource_id: 694", "请执行 create_alert_rule。参数如下：- group_id: 1 - datasource_id: 694", 694, true},
		{"id after 数据源 keyword", "用数据源 42 创建告警", 42, true},
		{"nested name picks longest", "数据源：prod-mysql 建个告警", 42, true},
		{"unknown id defers", "datasource_id: 9999", 0, false},
		{"stray number not adjacent", "阈值90% 持续10分钟", 0, false},
		{"no datasource mentioned", "P1,90,10分钟，无偏好", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := pickDatasource(dsList, c.input)
			if id != c.wantID || ok != c.wantOK {
				t.Fatalf("pickDatasource(%q) = (%d,%v), want (%d,%v)", c.input, id, ok, c.wantID, c.wantOK)
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
