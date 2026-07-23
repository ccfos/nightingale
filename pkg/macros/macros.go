package macros

import (
	"fmt"
	"regexp"
	"strings"
)

// Macro expands SQL macros ($__timeFilter, $__timeGroup, etc) for a given
// datasource. The datasourceType parameter is the same string constant that
// each datasource registers with datasource.RegisterDatasource (for example
// ck.CKType = "ck", es.ESType = "elasticsearch"). Reusing those existing
// constants keeps the "type name" definition single-sourced in each
// datasource package instead of duplicating them here.
var Macro func(sql string, start, end int64, datasourceType string) (string, error)

func RegisterMacro(f func(sql string, start, end int64, datasourceType string) (string, error)) {
	Macro = f
}

func MacroInVain(sql string, start, end int64, _ string) (string, error) {
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
// A window of 0..0 is refused instead. It means the caller never resolved a time
// range, and expanding it yields a predicate that is valid SQL yet matches
// nothing, so a periodic query would keep succeeding while silently returning an
// empty result. Reporting the error lets the caller log it and count it as a
// failure rather than mistaking the empty result for "no data".
//
// The FROM_UNIXTIME predicate is only valid for dialects that understand it, so
// expansion is limited to those datasource types; every other dialect gets its
// SQL back verbatim, macro included, exactly as MacroInVain would return it.
func ExpandTimeFilter(sql string, start, end int64, datasourceType string) (string, error) {
	if !strings.Contains(sql, timeFilterMacro) {
		return sql, nil
	}

	// Literals mirror mysql.MySQLType and doris.DorisType; importing those
	// packages here would create an import cycle, as they import this one.
	switch datasourceType {
	case "mysql", "doris":
	default:
		return sql, nil
	}

	if start == 0 && end == 0 {
		return "", fmt.Errorf("%s requires a query time range, got none", timeFilterMacro)
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
