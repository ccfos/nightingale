package macros

import (
	"fmt"
	"regexp"
	"strings"
)

var Macro func(sql string, start, end int64) (string, error)

func RegisterMacro(f func(sql string, start, end int64) (string, error)) {
	Macro = f
}

func MacroInVain(sql string, start, end int64) (string, error) {
	return sql, nil
}

// timeFilterMacro is the macro users write in SQL to reference the query time
// range, e.g. "WHERE $__timeFilter(`ts`)".
const timeFilterMacro = "$__timeFilter"

// timeFilterPattern captures the column expression inside $__timeFilter(...).
//
// The capture group deliberately rejects parentheses so that the pattern only
// matches a plain column reference such as `ts`, t.ts or `db`.`t`.`ts`. An
// expression containing a nested call (for example $__timeFilter(DATE(ts))) is
// left untouched rather than being mangled by a partial match.
var timeFilterPattern = regexp.MustCompile(`\$__timeFilter\(([^()]*)\)`)

// ExpandTimeFilter replaces every $__timeFilter(<column>) occurrence with an
// explicit time range predicate, so that a query can reference the time window
// the caller asks for instead of hard-coding it.
//
// start and end are Unix timestamps in seconds, and the generated predicate is
// half-open — the start bound is inclusive and the end bound is exclusive:
//
//	$__timeFilter(`ts`)
//	=> (`ts` >= FROM_UNIXTIME(1784300000) AND `ts` < FROM_UNIXTIME(1784300060))
//
// A half-open range keeps consecutive evaluations of a periodic query from
// counting a row that sits exactly on a window boundary twice.
//
// The result is always wrapped in parentheses so the predicate keeps its
// meaning when the surrounding SQL combines it with OR.
//
// The SQL is returned unchanged when it holds no macro, and any occurrence that
// does not carry a column expression is left as it is: emitting the original
// statement surfaces the mistake at query time, which is preferable to failing
// the whole evaluation here.
//
// FROM_UNIXTIME is understood by Doris and MySQL. Other dialects are not
// handled, because the signature carries no datasource type to switch on.
func ExpandTimeFilter(sql string, start, end int64) (string, error) {
	if !strings.Contains(sql, timeFilterMacro) {
		return sql, nil
	}

	expanded := timeFilterPattern.ReplaceAllStringFunc(sql, func(match string) string {
		groups := timeFilterPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}

		column := strings.TrimSpace(groups[1])
		if column == "" {
			return match
		}

		return fmt.Sprintf("(%s >= FROM_UNIXTIME(%d) AND %s < FROM_UNIXTIME(%d))",
			column, start, column, end)
	})

	return expanded, nil
}
