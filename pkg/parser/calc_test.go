package parser

import (
	"testing"
)

func TestMathCalc(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		data     map[string]interface{}
		expected float64
		wantErr  bool
	}{
		{
			name:     "Add and Subtract",
			expr:     "一个 + $.B - $.C",
			data:     map[string]interface{}{"一个": 1, "$.B": 2, "$.C": 3},
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "Multiply and Divide",
			expr:     "($A.err_count >0&& $A.err_count <=3)||($B.err_count>0 && $B.err_count <=5)",
			data:     map[string]interface{}{"$A.err_count": 4, "$B.err_count": 2},
			expected: 1,
			wantErr:  false,
		},
		{
			name:     "Subtract and Add",
			expr:     "$.C - $.D + $.A",
			data:     map[string]interface{}{"$.A": 5, "$.C": 3, "$.D": 2},
			expected: 6,
			wantErr:  false,
		},
		{
			name:     "Divide and Multiply",
			expr:     "$.B / $.C * $.D",
			data:     map[string]interface{}{"$.B": 6, "$.C": 2, "$.D": 3},
			expected: 9,
			wantErr:  false,
		},
		{
			name:     "Divide and Multiply",
			expr:     "$.B / $.C * $.D",
			data:     map[string]interface{}{"$.B": 6, "$.C": 2, "$.D": 3},
			expected: 9,
			wantErr:  false,
		},
		{
			name:     "Multiply and Add",
			expr:     "$.A * $.B + $.C",
			data:     map[string]interface{}{"$.A": 2, "$.B": 3, "$.C": 4},
			expected: 10,
			wantErr:  false,
		},
		{
			name:     "Subtract and Divide",
			expr:     "$.D - $.A / $.B",
			data:     map[string]interface{}{"$.D": 10, "$.A": 4, "$.B": 2},
			expected: 8,
			wantErr:  false,
		},
		{
			name:     "Add, Subtract and Subtract",
			expr:     "$.C + $.D - $.A",
			data:     map[string]interface{}{"$.C": 3, "$.D": 4, "$.A": 5},
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Multiply and Subtract",
			expr:     "$.B * $.A - $.D",
			data:     map[string]interface{}{"$.B": 2, "$.A": 3, "$.D": 4},
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Divide and Add",
			expr:     "$.A / $.B + $.C",
			data:     map[string]interface{}{"$.A": 4, "$.B": 2, "$.C": 3},
			expected: 5,
			wantErr:  false,
		},
		{
			name:     "Add and Multiply",
			expr:     "$.D + $.A * $.B",
			data:     map[string]interface{}{"$.D": 1, "$.A": 2, "$.B": 3},
			expected: 7,
			wantErr:  false,
		},
		{
			name:     "Divide and Add with Parentheses",
			expr:     "($A / $B) + ($C * $D)",
			data:     map[string]interface{}{"$A": 4, "$B": 2, "$C": 1, "$D": 3},
			expected: 5.0,
			wantErr:  false,
		},
		{
			name:     "Divide with Parentheses",
			expr:     "($.A - $.B) / ($.C + $.D)",
			data:     map[string]interface{}{"$.A": 6, "$.B": 2, "$.C": 3, "$.D": 1},
			expected: 1.0,
			wantErr:  false,
		},
		{
			name:     "Add and Multiply with Parentheses",
			expr:     "($.A + $.B) * ($.C - $.D)",
			data:     map[string]interface{}{"$.A": 8, "$.B": 2, "$.C": 4, "$.D": 2},
			expected: 20,
			wantErr:  false,
		},
		{
			name:     "Divide and Multiply with Parentheses",
			expr:     "($.A * $.B) / ($.C - $.D)",
			data:     map[string]interface{}{"$.A": 8, "$.B": 2, "$.C": 4, "$.D": 2},
			expected: 8,
			wantErr:  false,
		},
		{
			name:     "Add and Divide with Parentheses",
			expr:     "$.A + ($.B * $.C) / $.D",
			data:     map[string]interface{}{"$.A": 1, "$.B": 2, "$.C": 3, "$.D": 4},
			expected: 2.5,
			wantErr:  false,
		},
		{
			name:     "Subtract and Multiply with Parentheses",
			expr:     "($.A + $.B) - ($.C * $.D)",
			data:     map[string]interface{}{"$.A": 5, "$.B": 2, "$.C": 3, "$.D": 1},
			expected: 4,
			wantErr:  false,
		},
		{
			name:     "Multiply and Divide with Parentheses",
			expr:     "$.A / ($.B - $.C) * $.D",
			data:     map[string]interface{}{"$.A": 4, "$.B": 3, "$.C": 2, "$.D": 5},
			expected: 20.0,
			wantErr:  false,
		},
		{
			name:     "Multiply and Divide with Parentheses 2",
			expr:     "($.A - $.B) * ($.C / $.D)",
			data:     map[string]interface{}{"$.A": 3, "$.B": 1, "$.C": 2, "$.D": 4},
			expected: 1.0,
			wantErr:  false,
		},

		{
			name:     "Complex expression",
			expr:     "$.A/$.B*$.D",
			data:     map[string]interface{}{"$.A": 1, "$.B": 2, "$.C": 3, "$.D": 4},
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Complex expression",
			expr:     "$.A/$.B*$.C",
			data:     map[string]interface{}{"$.A": 2, "$.B": 2, "$.C": 2},
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Complex expression",
			expr:     "$.A/($.B*$.C)",
			data:     map[string]interface{}{"$.A": 2, "$.B": 2, "$.C": 2},
			expected: 0.5,
			wantErr:  false,
		},
		{
			name:     "Addition",
			expr:     "$.A + $.B",
			data:     map[string]interface{}{"$.A": 2, "$.B": 3},
			expected: 5,
			wantErr:  false,
		},
		{
			name:     "Subtraction",
			expr:     "$.A - $.B",
			data:     map[string]interface{}{"$.A": 5, "$.B": 3},
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Multiplication",
			expr:     "$.A * $.B",
			data:     map[string]interface{}{"$.A": 4, "$.B": 3},
			expected: 12,
			wantErr:  false,
		},
		{
			name:     "Division",
			expr:     "$.A / $.B",
			data:     map[string]interface{}{"$.A": 10, "$.B": 2},
			expected: 5,
			wantErr:  false,
		},
		{
			name:     "Mixed operations",
			expr:     "($.A + $.B) * ($.C - $.D)",
			data:     map[string]interface{}{"$.A": 1, "$.B": 2, "$.C": 5, "$.D": 3},
			expected: 6, // Corrected from 9 to 6
			wantErr:  false,
		},
		{
			name:     "Parentheses",
			expr:     "($.A + $.B) / ($.C - $.D)",
			data:     map[string]interface{}{"$.A": 6, "$.B": 4, "$.C": 10, "$.D": 2},
			expected: 1.25, // Corrected from 2.5 to 1.25
			wantErr:  false,
		},
		{
			name:     "Add and Multiply with Parentheses for float64 and int",
			expr:     "($.A + $.B) * ($.C - $.D)",
			data:     map[string]interface{}{"$.A": 8.0, "$.B": 2.0, "$.C": 4.0, "$.D": 2},
			expected: 20,
			wantErr:  false,
		},
		{
			name:     "Divide and Multiply with Parentheses for float64 and int",
			expr:     "($.A * $.B) / ($.C - $.D)",
			data:     map[string]interface{}{"$.A": 8, "$.B": 2, "$.C": 4.0, "$.D": 2},
			expected: 8,
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Run the MathCalc function
			result, err := MathCalc(tc.expr, tc.data)

			// Check for expected errors
			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected an error for expr '%s', but got none:%v", tc.expr, result)
				}
				return
			}

			// If an error is not expected, but occurs, fail the test
			if err != nil {
				t.Fatalf("Unexpected error for expr '%s' data:%v err:%v", tc.expr, tc.data, err)
			}

			// Compare the expected result with the actual result
			if result != tc.expected {
				t.Errorf("Expected result for expr '%s' to be %v, got %v", tc.expr, tc.expected, result)
			}
		})
	}
}

func TestCalc(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		data     map[string]interface{}
		expected bool
	}{
		{
			name:     "Greater than - true",
			expr:     "$.A > $.B",
			data:     map[string]interface{}{"$.A": 5, "$.B": 3},
			expected: true,
		},
		{
			name:     "Multiply and Subtract with Parentheses",
			expr:     "$A.yesterday_rate > 0.1 && $A.last_week_rate>0.1 or ($A.今天 >300 || $A.昨天>300 || $A.上周今天 > 300)",
			data:     map[string]interface{}{"$A.yesterday_rate": 0.1, "$A.last_week_rate": 2, "$A.今天": 200.4, "$A.昨天": 200.4, "$A.上周今天": 200.4},
			expected: false,
		},
		{
			name:     "Count Greater Than Zero with Code",
			expr:     "$A.count > 0",
			data:     map[string]interface{}{"$A.count": 197, "$A.code": 30000},
			expected: true,
		},
		{
			name:     "Today, Yesterday, and Lastweek Rate Comparison",
			expr:     "$A.todayRate<0.3 && $A.yesterdayRate<0.3 && $A.lastweekRate<0.3",
			data:     map[string]interface{}{"$A.todayRate": 1.1, "$A.yesterdayRate": 0.8, "$A.lastweekRate": 1.2},
			expected: false,
		},
		{
			name:     "Today, Yesterday, and Lastweek Rate Low Threshold",
			expr:     "$A.todayRate<0.1 && $A.yesterdayRate<0.1 && $A.lastweekRate<0.1",
			data:     map[string]interface{}{"$A.todayRate": 0.9, "$A.yesterdayRate": 0.8, "$A.lastweekRate": 0.9},
			expected: false,
		},
		{
			name:     "Agent Specific Today, Yesterday, and Lastweek Rate Comparison",
			expr:     "$A.agent == 11 && $A.todayRate<0.3 && $A.yesterdayRate<0.3 && $A.lastweekRate<0.3",
			data:     map[string]interface{}{"$A.agent": 11, "$A.todayRate": 0.9, "$A.yesterdayRate": 0.9, "$A.lastweekRate": 1},
			expected: false,
		},
		{
			name:     "Today, Yesterday, and Lastweek Rate Below 0.1 - Case 1",
			expr:     "$A<0.1 && $A.yesterdayRate<0.1 && $A.lastweekRate<0.1",
			data:     map[string]interface{}{"$A": 0.8, "$A.yesterdayRate": 0.9, "$A.lastweekRate": 0.9},
			expected: false,
		},
		{
			name:     "Today, Yesterday, and Lastweek Rate Below 0.1 - Case 2",
			expr:     "$A.today_rate<0.1 && $A.yesterday_rate<0.1 && $A.lastweek_rate<0.1",
			data:     map[string]interface{}{"$A.today_rate": 0.9, "$A.yesterday_rate": 0.9, "$A.lastweek_rate": 0.9},
			expected: false,
		},
		{
			name:     "Today, Yesterday, and Lastweek Rate Below 0.1 - Case 3",
			expr:     "$B.today_rate<0.1 && $A.yesterday_rate<0.1 && $A.lastweek_rate<0.1",
			data:     map[string]interface{}{"$B.today_rate": 0.5, "$A.yesterday_rate": 0.9, "$A.lastweek_rate": 0.8},
			expected: false,
		},
		{
			name:     "Yesterday and Byesterday Rates Logical Conditions - Case 1",
			expr:     "($A.yesterday_rate > 2 && $A.byesterday_rate > 2) or ($A.yesterday_rate <= 0.7 && $A.byesterday_rate <= 0.7)",
			data:     map[string]interface{}{"$A.yesterday_rate": 3, "$A.byesterday_rate": 3},
			expected: true,
		},
		{
			name:     "Yesterday and Byesterday Rates Higher Thresholds - Case 1",
			expr:     "($A.yesterday_rate > 1.5 && $A.byesterday_rate > 1.5) or ($A.yesterday_rate <= 0.8 && $A.byesterday_rate <= 0.8)",
			data:     map[string]interface{}{"$A.yesterday_rate": 1.08, "$A.byesterday_rate": 1.02},
			expected: false,
		},
		{
			name:     "Greater than - false",
			expr:     "($A.yesterday_rate > 1.0 && $A.byesterday_rate > 1.0 ) or ($A.yesterday_rate <= 0.9 && $A.byesterday_rate <= 0.9)",
			data:     map[string]interface{}{"$A.byesterday_rate": 0.33, "$A.yesterday_rate": 2},
			expected: false,
		},
		{
			name:     "Less than - true",
			expr:     "$A.count > 100 or $A.count2 > -3",
			data:     map[string]interface{}{"$A.count": 5, "$A.count2": -1, "$.D": 2},
			expected: true,
		},
		{
			name:     "Less than - false",
			expr:     "$.A < $.B/$.B*4",
			data:     map[string]interface{}{"$.A": 5, "$.B": 3},
			expected: false,
		},
		{
			name:     "Greater than or equal - true",
			expr:     "$.A >= $.B",
			data:     map[string]interface{}{"$.A": 3, "$.B": 3},
			expected: true,
		},
		{
			name:     "Less than or equal - true",
			expr:     "$.A <= $.B",
			data:     map[string]interface{}{"$.A": 2, "$.B": 2},
			expected: true,
		},
		{
			name:     "Not equal - true",
			expr:     "$.A != $.B",
			data:     map[string]interface{}{"$.A": 3, "$.B": 2},
			expected: true,
		},
		{
			name:     "Not equal - false",
			expr:     "$.A != $.B",
			data:     map[string]interface{}{"$.A": 2, "$.B": 2},
			expected: false,
		},
		{
			name:     "Addition resulting in true",
			expr:     "$.A + $.B > $.C",
			data:     map[string]interface{}{"$.A": 3, "$.B": 2, "$.C": 4},
			expected: true,
		},
		{
			name:     "Subtraction resulting in false",
			expr:     "$.A - $.B < $.C",
			data:     map[string]interface{}{"$.A": 1, "$.B": 3, "$.C": 1},
			expected: true,
		},
		{
			name:     "Multiplication resulting in true",
			expr:     "$.A * $.B > $.C",
			data:     map[string]interface{}{"$.A": 2, "$.B": 3, "$.C": 5},
			expected: true,
		},
		{
			name:     "Division resulting in false",
			expr:     "$.A / $.B*$.C < $.C",
			data:     map[string]interface{}{"$.A": 4, "$.B": 2, "$.C": 2},
			expected: false,
		},
		{
			name:     "Addition with parentheses resulting in true",
			expr:     "($.A + $.B) > $.C && $.A >0",
			data:     map[string]interface{}{"$.A": 1, "$.B": 4, "$.C": 4},
			expected: true,
		},
		{
			name:     "Addition with parentheses resulting in true",
			expr:     "($.A + $.B) > $.C || $.A < 0",
			data:     map[string]interface{}{"$.A": 1, "$.B": 4, "$.C": 4},
			expected: true,
		},
		{
			name:     "Complex expression with parentheses resulting in false",
			expr:     "($.A + $.B) * $.C < $.D",
			data:     map[string]interface{}{"$.A": 1, "$.B": 2, "$.C": 3, "$.D": 10},
			expected: true,
		},
		{
			name:     "Nested parentheses resulting in true",
			expr:     "($.A + ($.B - $.C)) * $.D > $.E",
			data:     map[string]interface{}{"$.A": 2, "$.B": 5, "$.C": 2, "$.D": 2, "$.E": 8},
			expected: true,
		},
		{
			name:     "Division with parentheses resulting in false",
			expr:     " ( true || false ) && true",
			data:     map[string]interface{}{"$A": 673601, "$A.": 673601, "$B": 250218, "$C": 456513, "$C.": 456513, "$D": 456513, "$D.": 456513},
			expected: true,
		},
		// $A:673601.5 $A.:673601.5 $B:361520 $B.:361520 $C:456513 $C.:456513 $D:422634 $D.:422634]

		{
			name:     "Greater than or equal for string - true",
			expr:     "$.A >= $.B",
			data:     map[string]interface{}{"$.A": "123", "$.B": "123"},
			expected: true,
		},
		{
			name:     "Less than or equal - true",
			expr:     "$.A <= $.B",
			data:     map[string]interface{}{"$.A": "abc", "$.B": "abc"},
			expected: true,
		},
		{
			name:     "Not equal - true",
			expr:     "$.A != $.B",
			data:     map[string]interface{}{"$.A": "abcde", "$.B": "abcdf"},
			expected: true,
		},
		{
			name:     "Not equal - false",
			expr:     "$.A != $.B",
			data:     map[string]interface{}{"$.A": "!@#$qwer1234", "$.B": "!@#$qwer1234"},
			expected: false,
		},
		{
			name:     "In operation for string resulting in false",
			expr:     `$.A in ["admin", "moderator"]`,
			data:     map[string]interface{}{"$.A": "admin1"},
			expected: false,
		},
		{
			name:     "In operation for string resulting in true",
			expr:     `$.A in ["admin", "moderator"]`,
			data:     map[string]interface{}{"$.A": "admin"},
			expected: true,
		},
		{
			name:     "In operation for int resulting in false",
			expr:     `$.A not in [1, 2, 3]`,
			data:     map[string]interface{}{"$.A": 2},
			expected: false,
		},
		{
			name:     "In operation for int resulting in true",
			expr:     `$.A not in [1, 2, 3]`,
			data:     map[string]interface{}{"$.A": 5},
			expected: true,
		},
		{
			name:     "Contains operation resulting in true",
			expr:     `$.A contains $.B`,
			data:     map[string]interface{}{"$.A": "hello world", "$.B": "world"},
			expected: true,
		},
		{
			name:     "Contains operation resulting in false",
			expr:     `$.A contains $.B`,
			data:     map[string]interface{}{"$.A": "hello world", "$.B": "go"},
			expected: false,
		},
		{
			name:     "Contains operation resulting in false",
			expr:     `$.A not contains $.B`,
			data:     map[string]interface{}{"$.A": "hello world", "$.B": "world"},
			expected: false,
		},
		{
			name:     "Contains operation resulting in true",
			expr:     `$.A not contains $.B`,
			data:     map[string]interface{}{"$.A": "hello world", "$.B": "go"},
			expected: true,
		},
		{
			name:     "regex operation resulting in true",
			expr:     `$.A matches $.B`,
			data:     map[string]interface{}{"$.A": "123", "$.B": "^[0-9]+$"},
			expected: true,
		},
		{
			name:     "regex operation resulting in false",
			expr:     `$.A matches $.B`,
			data:     map[string]interface{}{"$.A": "abc", "$.B": "^[0-9]+$"},
			expected: false,
		},
		{
			name:     "between function resulting in true",
			expr:     `between($.A, [100,200])`,
			data:     map[string]interface{}{"$.A": 155.0},
			expected: true,
		},
		{
			name:     "between function resulting in false",
			expr:     `not between($.A, [100.3,200.3])`,
			data:     map[string]interface{}{"$.A": 155.1},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Calc(tc.expr, tc.data)
			if result != tc.expected {
				t.Errorf("Expected result for expr '%s' to be %v, got %v", tc.expr, tc.expected, result)
			}
		})
	}
}

// TestCalcDegrade 覆盖子条件因变量缺失/无数据求值报错时按布尔逻辑降级为 false 的行为。
func TestCalcDegrade(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		data     map[string]interface{}
		expected bool
	}{
		// OR：报错项当 false 跳过，其它项为真即触发
		{
			name:     "OR with nodata, A true -> trigger",
			expr:     "$A || $B",
			data:     map[string]interface{}{"$A": 1},
			expected: true,
		},
		{
			name:     "OR with nodata, A false -> not trigger",
			expr:     "$A || $B",
			data:     map[string]interface{}{"$A": 0},
			expected: false,
		},
		{
			name:     "OR multi with nodata, one true -> trigger",
			expr:     "$A || $B || $C",
			data:     map[string]interface{}{"$C": 1},
			expected: true,
		},
		// AND：报错项当 false，使整条为 false
		{
			name:     "AND with nodata, A true -> not trigger",
			expr:     "$A && $B",
			data:     map[string]interface{}{"$A": 1},
			expected: false,
		},
		{
			name:     "AND all present true -> trigger",
			expr:     "$A && $B",
			data:     map[string]interface{}{"$A": 1, "$B": 1},
			expected: true,
		},
		// 混合：&& 紧于 ||
		{
			name:     "Mixed A||B&&C, B nodata, A true -> trigger",
			expr:     "$A || $B && $C",
			data:     map[string]interface{}{"$A": 1, "$C": 1},
			expected: true,
		},
		{
			name:     "Mixed A||B&&C, A nodata, B&&C true -> degrade to B&&C true",
			expr:     "$A || $B && $C",
			data:     map[string]interface{}{"$B": 1, "$C": 1},
			expected: true,
		},
		{
			name:     "Mixed A||B&&C, A nodata, B&&C false -> degrade to B&&C false",
			expr:     "$A || $B && $C",
			data:     map[string]interface{}{"$B": 1, "$C": 0},
			expected: false,
		},
		// 取反：作用在无数据子条件上 -> 保守不触发
		{
			name:     "NOT nodata -> not trigger",
			expr:     "!$A",
			data:     map[string]interface{}{},
			expected: false,
		},
		{
			name:     "NOT present false -> trigger",
			expr:     "!$A",
			data:     map[string]interface{}{"$A": 0},
			expected: true,
		},
		{
			name:     "NOT group with nodata operand, other false -> conservative not trigger",
			expr:     "!($A || $B)",
			data:     map[string]interface{}{"$B": 0},
			expected: false,
		},
		{
			name:     "NOT group, other true -> not trigger",
			expr:     "!($A || $B)",
			data:     map[string]interface{}{"$B": 1},
			expected: false,
		},
		{
			name:     "not word form on nodata -> not trigger",
			expr:     "not $A",
			data:     map[string]interface{}{},
			expected: false,
		},
		// 全部子条件报错 -> 整条 false，不 panic
		{
			name:     "OR all nodata -> false",
			expr:     "$A || $B || $C",
			data:     map[string]interface{}{},
			expected: false,
		},
		{
			name:     "AND all nodata -> false",
			expr:     "$A && $B && $C",
			data:     map[string]interface{}{},
			expected: false,
		},
		// 子条件粒度降级（带比较与括号）
		{
			name:     "grouped AND nodata in one OR branch, other branch triggers",
			expr:     "($A.x > 0 && $A.y > 0) || ($B.x > 0)",
			data:     map[string]interface{}{"$A.x": 1, "$A.y": 1},
			expected: true,
		},
		{
			name:     "grouped AND with nodata label degrades whole branch to false",
			expr:     "($A.x > 0 && $A.y > 0) || ($B.x > 0)",
			data:     map[string]interface{}{"$A.x": 1, "$B.x": 0},
			expected: false,
		},
		{
			name:     "or word form, nodata branch skipped",
			expr:     "$A.cnt > 0 or $B.cnt > 0",
			data:     map[string]interface{}{"$A.cnt": 5},
			expected: true,
		},
		// 语法错误 -> 按现状判 false（不降级）
		{
			name:     "syntax error -> false",
			expr:     "$A >",
			data:     map[string]interface{}{"$A": 1},
			expected: false,
		},
		{
			name:     "trailing operator -> false",
			expr:     "$A && ",
			data:     map[string]interface{}{"$A": 1},
			expected: false,
		},
		// 短路语义：左侧已定结果时不求值右侧，避免本不该执行的运行期错误（如越界）
		{
			name:     "OR short-circuit avoids right runtime error",
			expr:     "$.A > 0 || $.Arr[0] > 0",
			data:     map[string]interface{}{"$.A": 1, "$.Arr": []interface{}{}},
			expected: true,
		},
		{
			name:     "AND short-circuit avoids right runtime error",
			expr:     "$.A > 0 && $.Arr[0] > 0",
			data:     map[string]interface{}{"$.A": 0, "$.Arr": []interface{}{}},
			expected: false,
		},
		{
			name:     "runtime error on evaluated branch -> hard error false",
			expr:     "$.A > 0 && $.Arr[0] > 0",
			data:     map[string]interface{}{"$.A": 1, "$.Arr": []interface{}{}},
			expected: false,
		},
		// 未知函数（拼写错误）不应被当成无数据降级，整条按硬错误判 false
		{
			name:     "unknown function not degraded, other branch true -> false",
			expr:     "betwen($.A, [1,2]) || $.B > 0",
			data:     map[string]interface{}{"$.A": 1, "$.B": 1},
			expected: false,
		},
		{
			name:     "unknown function in short-circuited OR branch -> false",
			expr:     "$.B > 0 || betwen($.A, [1,2])",
			data:     map[string]interface{}{"$.A": 1.5, "$.B": 1},
			expected: false,
		},
		{
			name:     "syntax error in short-circuited OR branch -> false",
			expr:     "$.B > 0 || $.A >",
			data:     map[string]interface{}{"$.A": 1, "$.B": 1},
			expected: false,
		},
		{
			name:     "valid between still works",
			expr:     "between($.A, [1,2]) || $.B > 0",
			data:     map[string]interface{}{"$.A": 1.5, "$.B": 0},
			expected: true,
		},
		// 反引号字符串：其中的 or/and 不应被误切
		{
			name:     "backtick string with or not split",
			expr:     "$.A == `foo or bar` || $.B > 0",
			data:     map[string]interface{}{"$.A": "foo or bar", "$.B": 0},
			expected: true,
		},
		{
			name:     "backtick string with and not split",
			expr:     "$.A == `x and y` && $.B > 0",
			data:     map[string]interface{}{"$.A": "x and y", "$.B": 1},
			expected: true,
		},
		// 三元表达式：分支/条件里的 || && 不是顶层逻辑，整体交给 MathCalc
		{
			name:     "ternary with OR in true branch",
			expr:     "$.C ? $.A > 0 || $.B > 0 : false",
			data:     map[string]interface{}{"$.C": true, "$.A": 0, "$.B": 1},
			expected: true,
		},
		{
			name:     "ternary false branch decides",
			expr:     "$.C ? $.A > 0 : $.B > 0",
			data:     map[string]interface{}{"$.C": false, "$.A": 0, "$.B": 1},
			expected: true,
		},
		{
			name:     "parenthesized ternary still allows OR split and degrade",
			expr:     "($.A > 0 ? 1 : 0) || $.B > 0",
			data:     map[string]interface{}{"$.A": 1},
			expected: true,
		},
		// 未加括号混用 ?? 与 ||/&&：expr 视为整体语法错误，切分不应绕过该校验
		{
			name:     "coalesce mixed with OR without parens -> false",
			expr:     "$.A ?? $.B || $.C",
			data:     map[string]interface{}{"$.A": 1, "$.B": 1, "$.C": 1},
			expected: false,
		},
		{
			name:     "coalesce mixed with AND without parens -> false",
			expr:     "$.A ?? $.B && $.C",
			data:     map[string]interface{}{"$.A": 1, "$.B": 1, "$.C": 1},
			expected: false,
		},
		{
			name:     "coalesce parenthesized with OR -> valid",
			expr:     "($.A ?? $.B) > 0 || $.C > 0",
			data:     map[string]interface{}{"$.A": 1, "$.B": 0, "$.C": 0},
			expected: true,
		},
		// 有数据时与改造前一致（回归）
		{
			name:     "regression: comparison OR with data",
			expr:     "$A.count > 100 or $A.count2 > -3",
			data:     map[string]interface{}{"$A.count": 5, "$A.count2": -1},
			expected: true,
		},
		{
			name:     "regression: nested parens with data",
			expr:     "($A.yesterday_rate > 2 && $A.byesterday_rate > 2) or ($A.yesterday_rate <= 0.7 && $A.byesterday_rate <= 0.7)",
			data:     map[string]interface{}{"$A.yesterday_rate": 3, "$A.byesterday_rate": 3},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Calc(tc.expr, tc.data)
			if result != tc.expected {
				t.Errorf("Expected result for expr '%s' (data:%v) to be %v, got %v", tc.expr, tc.data, tc.expected, result)
			}
			// CalcWithRid 共用同一套求值逻辑，结果应保持一致
			if r := CalcWithRid(tc.expr, tc.data, 1); r != tc.expected {
				t.Errorf("CalcWithRid result for expr '%s' (data:%v) to be %v, got %v", tc.expr, tc.data, tc.expected, r)
			}
		})
	}
}
