package sqlbase

import (
	"strings"
	"testing"
)

// 必须拒绝：各类绕过 BannedOp 空格切词黑名单的写操作形态
func TestValidateReadOnlyRejectsWrites(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		// BannedOp 用空格切词，以下分隔符都能绕过它，必须被这里拦住
		{"tab separated delete", "DELETE\tFROM users WHERE id = 1"},
		{"newline separated delete", "DELETE\nFROM users"},
		{"crlf separated drop", "DROP\r\nTABLE users"},
		{"paren wrapped delete", "(DELETE FROM users)"},
		{"multiple spaces then delete", "DELETE  FROM users"},

		// 注释拼接
		{"block comment inside keyword", "DEL/**/ETE FROM users"},
		{"leading comment then write", "/* harmless */ DELETE FROM users"},
		{"line comment then write", "-- c\nDROP TABLE users"},

		// 多语句串联
		{"stacked drop", "SELECT 1; DROP TABLE users"},
		{"stacked delete with trailing semicolon", "SELECT 1; DELETE FROM users;"},

		// PG data-modifying CTE：以 WITH 开头，起始关键字检查放行，靠词边界扫描拦
		{"data modifying cte", "WITH t AS (DELETE FROM foo RETURNING *) SELECT * FROM t"},
		{"data modifying cte insert", "WITH t AS (INSERT INTO foo VALUES (1) RETURNING *) SELECT * FROM t"},

		// 落盘 / 提权 / 状态变更
		{"select into outfile", "SELECT * FROM users INTO OUTFILE '/tmp/x'"},
		{"select into dumpfile", "SELECT * FROM users INTO DUMPFILE '/tmp/x'"},
		{"grant", "GRANT ALL ON *.* TO 'x'@'%'"},
		{"set variable", "SET GLOBAL general_log = 1"},
		{"truncate", "TRUNCATE TABLE users"},
		{"insert", "INSERT INTO users VALUES (1)"},
		{"update", "UPDATE users SET name = 'x'"},
		{"ck attach", "ATTACH TABLE users"},
		{"ck optimize", "OPTIMIZE TABLE users"},
		{"call procedure", "CALL do_something()"},
		{"load data", "LOAD DATA INFILE '/tmp/x' INTO TABLE users"},

		// MySQL/MariaDB/Doris 可执行注释：内容会被服务端当正文执行，
		// 按普通注释剥掉会让校验器看到的骨架与实际执行的语句不一致
		{"executable comment outfile", "SELECT 1 /*!50000 INTO OUTFILE '/tmp/x' */"},
		{"executable comment mariadb", "SELECT 1 /*M!100000 INTO OUTFILE '/tmp/x' */"},
		{"executable comment no version", "SELECT 1 /*! DROP TABLE users */"},
		{"executable comment union", "SELECT * FROM t /*!50000 UNION SELECT * FROM mysql.user */"},

		// 裸 SELECT ... INTO：PG / SQL Server / Redshift 上是建表写操作，
		// 起始关键字是 SELECT，只能靠全句关键字扫描拦住
		{"select into table", "SELECT * INTO pwn FROM source"},
		{"select into with where", "SELECT a,b INTO newtbl FROM t WHERE x = 1"},
		{"select into temp table", "SELECT * INTO TEMP TABLE tmp1 FROM t"},
		{"cte select into", "WITH x AS (SELECT 1) SELECT * INTO y FROM x"},

		// 同时是函数名的关键字，语句形态仍须拒绝
		{"replace into", "REPLACE INTO users VALUES (1)"},
		{"merge into", "MERGE INTO t USING s ON t.id = s.id"},

		// 空语句
		{"empty", "   "},
		{"comment only", "/* nothing */"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateReadOnly(tt.sql); err == nil {
				t.Errorf("expected rejection for %q", tt.sql)
			}
		})
	}
}

// 必须放行：仪表盘面板与变量取值的常见只读查询，包括字面量里含关键字的场景
func TestValidateReadOnlyAllowsReads(t *testing.T) {
	cases := []struct {
		name string
		sql  string
	}{
		{"plain select", "SELECT ts, value FROM metrics WHERE host = 'web01'"},
		{"select with newlines", "SELECT ts,\n  value\nFROM metrics\nWHERE ts > 1755500000"},
		{"read only cte", "WITH t AS (SELECT * FROM metrics) SELECT count(*) FROM t"},
		{"union in parens", "(SELECT a FROM t1) UNION ALL (SELECT a FROM t2)"},
		{"show tables", "SHOW TABLES"},
		{"describe", "DESC metrics"},
		{"explain select", "EXPLAIN SELECT * FROM metrics"},
		{"trailing semicolon", "SELECT 1;"},

		// 字符串字面量里的关键字与分号不应触发判定（否则大面积错杀）
		{"keyword inside string literal", "SELECT * FROM audit WHERE action = 'delete'"},
		{"update word in literal", "SELECT * FROM audit WHERE msg = 'update failed'"},
		{"semicolon inside literal", "SELECT * FROM t WHERE note = 'a;b'"},
		{"escaped quote in literal", "SELECT * FROM t WHERE note = 'it''s a drop test'"},
		{"backslash escaped quote", `SELECT * FROM t WHERE note = 'x\' drop table y'`},

		// 关键字作为标识符前缀/后缀的一部分，不应被词边界误伤
		{"column named deleted_at", "SELECT deleted_at FROM users"},
		{"table named updates", "SELECT * FROM updates"},
		{"ck settings modifier", "SELECT * FROM metrics SETTINGS max_rows_to_read = 100"},

		// 既是语句关键字又是标准函数名/常见列名的词，函数与列名形态必须放行，
		// 否则分享链接下面板报错、登录态却正常，排查成本极高
		{"replace function", "SELECT REPLACE(name, 'a', 'b') FROM t"},
		{"replace function lowercase", "SELECT replace(host, '-', '_') AS h FROM metrics"},
		{"column named load", "SELECT ts, load FROM metrics"},
		{"ck merge table function", "SELECT merge(x) FROM t"},
		{"column named copy_count", "SELECT copy_count FROM t"},
		{"column named call_count", "SELECT call_count FROM t"},

		// 变量插值后的常见形态（多选变量拼成正则/IN 列表）
		{"multi value variable", "SELECT * FROM t WHERE host IN ('a','b','c') AND ts > 1755500000"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateReadOnly(tt.sql); err != nil {
				t.Errorf("expected pass for %q, got %v", tt.sql, err)
			}
		})
	}
}

func TestStripSQLComments(t *testing.T) {
	// 注释内容被剥离，字符串字面量里的注释符号保持原样
	got := stripSQLComments("SELECT 1 /* c */ FROM t -- tail\nWHERE x = 'a -- b'")
	if strings.Contains(got, "c") && !strings.Contains(got, "'a -- b'") {
		t.Errorf("comment stripping damaged string literal: %q", got)
	}
	if strings.Contains(got, "tail") {
		t.Errorf("line comment not stripped: %q", got)
	}
}
