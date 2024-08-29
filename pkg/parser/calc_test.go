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
