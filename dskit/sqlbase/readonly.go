package sqlbase

// 严格只读校验，用于不可信调用方（当前是仪表盘匿名分享 token）执行 SQL 数据源查询。
//
// 各方言原有的 BannedOp / ckBannedOp / DorisBannedOp 是按空格切词的关键字黑名单，
// 不是安全边界：`DELETE\tFROM`、`DELETE\nFROM`、`(DELETE` 都切不出被禁词。这里在
// 那层之外提供一个可靠得多的校验：剥掉注释后要求语句以只读关键字开头、拒绝多语句、
// 再用词边界匹配兜底扫描写操作关键字。
//
// 这不是完整的 SQL 解析器，仍是保守的语法层防线：它宁可错杀（拒绝合法但形态罕见的
// 查询）也不放行可疑语句。真正的纵深防御是给数据源配只读账号。

import (
	"fmt"
	"regexp"
	"strings"
)

// 允许的语句起始关键字。SELECT/WITH 是查询，SHOW/DESC/DESCRIBE/EXPLAIN 是元数据读取，
// 面板与变量取值都落在这几类里。
var readOnlyLeadingKeywords = map[string]struct{}{
	"SELECT":   {},
	"WITH":     {},
	"SHOW":     {},
	"DESC":     {},
	"DESCRIBE": {},
	"EXPLAIN":  {},
	"VALUES":   {},
	"TABLE":    {},
}

// 写/管理类关键字，用词边界匹配全句扫描——覆盖 PG 的 data-modifying CTE
// （`WITH t AS (DELETE FROM foo RETURNING *) SELECT * FROM t` 以 WITH 开头，
// 起始关键字检查放行，只能靠这里拦），以及 `SELECT ... INTO OUTFILE` 落盘。
var writeKeywordRe = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|RENAME|REPLACE|GRANT|REVOKE|ATTACH|DETACH|OPTIMIZE|CALL|EXEC|EXECUTE|MERGE|UPSERT|LOAD|COPY|OUTFILE|DUMPFILE|INTO\s+OUTFILE|INTO\s+DUMPFILE|LOCK|UNLOCK|SET|USE|KILL|SHUTDOWN|RESTORE|BACKUP)\b`)

// 允许出现在只读语句里的例外：SETTINGS 是 ClickHouse 查询修饰符（会被 \bSET\b 误伤
// 需靠词边界区分，这里显式说明）、SET 在 `SETTINGS` 中不构成独立词，无需额外处理。

// ValidateReadOnly 对不可信调用方的 SQL 做严格只读校验。通过返回 nil。
func ValidateReadOnly(sql string) error {
	// 先剥注释，再把字符串/标识符字面量掩成等长空白：字面量里的分号
	// （`SELECT 'a;b'`）和关键字（`WHERE action = 'delete'`）都不该触发判定，
	// 否则会大面积错杀合法查询。
	stripped := maskQuotedLiterals(stripSQLComments(sql))
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return fmt.Errorf("empty sql is not allowed")
	}

	// 拒绝多语句：去掉结尾分号后，句中不允许再出现分号，
	// 挡住 `SELECT 1; DROP TABLE x` 这类串联
	body := strings.TrimRight(trimmed, "; \t\r\n")
	if strings.Contains(body, ";") {
		return fmt.Errorf("multiple statements are not allowed in read-only mode")
	}

	// 起始关键字白名单。允许前导左括号（`(SELECT ...) UNION ...`）
	leading := strings.ToUpper(firstWord(strings.TrimLeft(body, "( \t\r\n")))
	if _, ok := readOnlyLeadingKeywords[leading]; !ok {
		return fmt.Errorf("only read-only statements are allowed in read-only mode, got %q", leading)
	}

	// 全句扫描写操作关键字（词边界），兜住 CTE 内嵌写、INTO OUTFILE 等
	if m := writeKeywordRe.FindString(body); m != "" {
		return fmt.Errorf("operation %s is forbidden in read-only mode", strings.ToUpper(m))
	}

	return nil
}

// firstWord 返回 s 的首个词（按空白切分）
func firstWord(s string) string {
	for i, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n', '(':
			if i == 0 {
				continue
			}
			return s[:i]
		}
	}
	return s
}

// maskQuotedLiterals 把单引号字符串、双引号/反引号标识符的内容替换成等长空格，
// 只保留引号本身。判定只看掩码后的骨架，字面量内容不参与关键字与分号检测。
// 未闭合的引号视为可疑：整段掩掉，交由后续起始关键字/关键字扫描决定。
func maskQuotedLiterals(sql string) string {
	b := []byte(sql)
	out := make([]byte, 0, len(b))

	for i := 0; i < len(b); i++ {
		c := b[i]
		if c != '\'' && c != '"' && c != '`' {
			out = append(out, c)
			continue
		}

		quote := c
		out = append(out, c)
		i++
		for i < len(b) {
			if b[i] == '\\' && quote != '`' && i+1 < len(b) {
				out = append(out, ' ', ' ')
				i += 2
				continue
			}
			// SQL 标准的引号转义：'' / "" / `` 表示字面引号，仍在串内
			if b[i] == quote {
				if i+1 < len(b) && b[i+1] == quote {
					out = append(out, ' ', ' ')
					i += 2
					continue
				}
				out = append(out, quote)
				break
			}
			out = append(out, ' ')
			i++
		}
	}

	return string(out)
}

// stripSQLComments 去掉 SQL 注释，避免 `DEL/**/ETE`、`--` 之后藏写操作等
// 形态骗过关键字检查。字符串字面量内的注释符号需要原样保留，否则会破坏语义。
func stripSQLComments(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))

	var inSingle, inDouble, inBacktick bool
	for i := 0; i < len(sql); i++ {
		c := sql[i]

		if inSingle {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(sql) { // 转义字符，连同下一个字节原样保留
				i++
				b.WriteByte(sql[i])
			} else if c == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(sql) {
				i++
				b.WriteByte(sql[i])
			} else if c == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			b.WriteByte(c)
			if c == '`' {
				inBacktick = false
			}
			continue
		}

		switch c {
		case '\'':
			inSingle = true
			b.WriteByte(c)
			continue
		case '"':
			inDouble = true
			b.WriteByte(c)
			continue
		case '`':
			inBacktick = true
			b.WriteByte(c)
			continue
		}

		// 行注释 -- 或 #，到行尾
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
			continue
		}
		if c == '#' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte(' ')
			continue
		}

		// 块注释 /* ... */，替换成空格，防止 DEL/**/ETE 拼回关键字
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			i++ // 跳到 '/'
			b.WriteByte(' ')
			continue
		}

		b.WriteByte(c)
	}

	return b.String()
}
