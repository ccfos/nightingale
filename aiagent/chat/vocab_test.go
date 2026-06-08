package chat

import "testing"

// TestHasImportIntent 锁定导入快路径的边界：integration 规则包/仪表盘模板导入归入
// creation（弹业务组+数据源表单），但批量 YAML/URL 导入与"看已导入"必须退回 general_chat。
func TestHasImportIntent(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		// 命中：integration 规则包/模板导入
		{"导入告警规则包", "从 integrations 导入 Redis 告警规则", true},
		{"导入+列出不被反信号挡掉", "帮用户导入 Redis 告警规则，先列出有哪些规则包供选择", true},
		{"导入仪表盘模板", "导入一个 Linux 仪表盘模板", true},
		{"import 英文", "import the redis alert rule pack", true},

		// 不命中：无导入动词
		{"纯创建无导入动词", "创建一条 CPU 告警规则", false},
		{"纯查询", "有哪些告警规则", false},

		// 不命中：导入动词但无可创建资源关键词
		{"导入但无 skill 关键词", "导入这份配置", false},

		// 不命中：prom-rule YAML/URL 批量导入 → 另一个 skill，工具不在 creation
		{"yml 文件导入", "帮我导入这个 node-exporter.yml 里的告警", false},
		{"url 导入", "导入 https://example.com/redis.yaml 的告警规则", false},
		{"批量导入", "批量导入一组 redis 告警规则", false},

		// 不命中：看已导入的东西
		{"查看已导入", "查看已导入的告警规则", false},
		{"导入历史", "看下告警规则的导入历史", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasImportIntent(c.input); got != c.want {
				t.Fatalf("HasImportIntent(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}
