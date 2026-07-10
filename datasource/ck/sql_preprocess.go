package ck

import (
	"github.com/ccfos/nightingale/v6/pkg/macros"
)

// SQLPreprocess is called before SQL execution to expand macros.
// Defaults to macros.Macro. Downstream projects (e.g. n9e-plus)
// can override this via RegisterSQLPreprocess to adapt macro
// dialects for ClickHouse.
var SQLPreprocess func(sql string, from, to int64) (string, error)

// RegisterSQLPreprocess sets a custom SQL preprocessor for ClickHouse SQL
// macro expansion, replacing the default macros.Macro delegation.
func RegisterSQLPreprocess(f func(sql string, from, to int64) (string, error)) {
	SQLPreprocess = f
}

func defaultSQLPreprocess(sql string, from, to int64) (string, error) {
	if macros.Macro != nil {
		return macros.Macro(sql, from, to)
	}
	return sql, nil
}
