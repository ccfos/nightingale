package macros

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandTimeFilter(t *testing.T) {
	const (
		start int64 = 1784300000
		end   int64 = 1784300060
	)

	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "backquoted column",
			sql:  "SELECT count(*) AS cnt FROM db.app_log WHERE $__timeFilter(`ts`)",
			want: "SELECT count(*) AS cnt FROM db.app_log WHERE " +
				"(`ts` >= FROM_UNIXTIME(1784300000) AND `ts` < FROM_UNIXTIME(1784300060))",
		},
		{
			name: "bare column",
			sql:  "SELECT count(*) FROM t WHERE $__timeFilter(ts)",
			want: "SELECT count(*) FROM t WHERE " +
				"(ts >= FROM_UNIXTIME(1784300000) AND ts < FROM_UNIXTIME(1784300060))",
		},
		{
			name: "table qualified column is kept verbatim",
			sql:  "SELECT count(*) FROM t WHERE $__timeFilter(t.ts)",
			want: "SELECT count(*) FROM t WHERE " +
				"(t.ts >= FROM_UNIXTIME(1784300000) AND t.ts < FROM_UNIXTIME(1784300060))",
		},
		{
			name: "surrounding whitespace is trimmed",
			sql:  "SELECT 1 WHERE $__timeFilter(  `ts`  )",
			want: "SELECT 1 WHERE (`ts` >= FROM_UNIXTIME(1784300000) AND `ts` < FROM_UNIXTIME(1784300060))",
		},
		{
			name: "every occurrence is expanded",
			sql: "SELECT * FROM (SELECT service FROM a WHERE $__timeFilter(`ts`)) x " +
				"JOIN b ON 1=1 WHERE $__timeFilter(`b`.`ts`)",
			want: "SELECT * FROM (SELECT service FROM a WHERE " +
				"(`ts` >= FROM_UNIXTIME(1784300000) AND `ts` < FROM_UNIXTIME(1784300060))) x " +
				"JOIN b ON 1=1 WHERE " +
				"(`b`.`ts` >= FROM_UNIXTIME(1784300000) AND `b`.`ts` < FROM_UNIXTIME(1784300060))",
		},
		{
			name: "predicate stays grouped next to OR",
			sql:  "SELECT 1 WHERE level = 'ERROR' OR $__timeFilter(`ts`)",
			want: "SELECT 1 WHERE level = 'ERROR' OR " +
				"(`ts` >= FROM_UNIXTIME(1784300000) AND `ts` < FROM_UNIXTIME(1784300060))",
		},
		{
			name: "sql without the macro is untouched",
			sql:  "SELECT count(*) FROM t WHERE ts >= DATE_SUB(NOW(), INTERVAL 5 MINUTE)",
			want: "SELECT count(*) FROM t WHERE ts >= DATE_SUB(NOW(), INTERVAL 5 MINUTE)",
		},
		{
			name: "empty column expression is left alone",
			sql:  "SELECT 1 WHERE $__timeFilter()",
			want: "SELECT 1 WHERE $__timeFilter()",
		},
		{
			name: "blank column expression is left alone",
			sql:  "SELECT 1 WHERE $__timeFilter(   )",
			want: "SELECT 1 WHERE $__timeFilter(   )",
		},
		{
			name: "nested call is left alone rather than partially matched",
			sql:  "SELECT 1 WHERE $__timeFilter(DATE(ts))",
			want: "SELECT 1 WHERE $__timeFilter(DATE(ts))",
		},
		{
			name: "unrelated macros are not touched",
			sql:  "SELECT $__timeGroup(ts, $__interval) FROM t",
			want: "SELECT $__timeGroup(ts, $__interval) FROM t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTimeFilter(tt.sql, start, end)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A caller that never resolved a time window would otherwise get a predicate
// that matches nothing, which reads as "no data" instead of as a failure.
func TestExpandTimeFilterRejectsAnEmptyWindow(t *testing.T) {
	_, err := ExpandTimeFilter("SELECT count(*) FROM t WHERE $__timeFilter(`ts`)", 0, 0)
	assert.Error(t, err)

	// The guard only speaks for statements that ask for the window.
	sql := "SELECT count(*) FROM t WHERE ts >= DATE_SUB(NOW(), INTERVAL 5 MINUTE)"
	got, err := ExpandTimeFilter(sql, 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, sql, got)

	// A window that merely starts at the epoch is a range, not a missing one.
	got, err = ExpandTimeFilter("SELECT 1 WHERE $__timeFilter(`ts`)", 0, 1784300060)
	assert.NoError(t, err)
	assert.Equal(t,
		"SELECT 1 WHERE (`ts` >= FROM_UNIXTIME(0) AND `ts` < FROM_UNIXTIME(1784300060))",
		got)
}

// The macro is registered through RegisterMacro at startup, so it has to keep
// satisfying the signature Macro is declared with.
func TestExpandTimeFilterIsRegistrable(t *testing.T) {
	prev := Macro
	defer func() { Macro = prev }()

	RegisterMacro(ExpandTimeFilter)
	assert.NotNil(t, Macro)

	got, err := Macro("SELECT 1 WHERE $__timeFilter(`ts`)", 1784300000, 1784300060)
	assert.NoError(t, err)
	assert.Equal(t,
		"SELECT 1 WHERE (`ts` >= FROM_UNIXTIME(1784300000) AND `ts` < FROM_UNIXTIME(1784300060))",
		got)
}

func TestMacroInVainKeepsSQLUnchanged(t *testing.T) {
	sql := "SELECT 1 WHERE $__timeFilter(`ts`)"

	got, err := MacroInVain(sql, 1784300000, 1784300060)
	assert.NoError(t, err)
	assert.Equal(t, sql, got)
}
