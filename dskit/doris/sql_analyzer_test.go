package doris

import (
	"testing"
)

func TestAnalyzeSQL_AggregateQueries(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		wantHasAgg  bool
		wantIsSelect bool
	}{
		// Standard aggregate functions - should skip check
		{
			name:        "COUNT(*)",
			sql:         "SELECT COUNT(*) FROM users",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "COUNT with column",
			sql:         "SELECT COUNT(id) FROM users",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "SUM function",
			sql:         "SELECT SUM(amount) FROM orders",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "AVG function",
			sql:         "SELECT AVG(price) FROM products",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "MIN function",
			sql:         "SELECT MIN(created_at) FROM logs",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "MAX function",
			sql:         "SELECT MAX(score) FROM results",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "Multiple aggregates",
			sql:         "SELECT COUNT(*), SUM(amount), AVG(price) FROM orders",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "Aggregate with GROUP BY",
			sql:         "SELECT user_id, COUNT(*) FROM orders GROUP BY user_id",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "Aggregate with WHERE and GROUP BY",
			sql:         "SELECT category, SUM(sales) FROM products WHERE status = 'active' GROUP BY category",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		{
			name:        "Aggregate with HAVING",
			sql:         "SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id HAVING cnt > 10",
			wantHasAgg:  true,
			wantIsSelect: true,
		},
		// Non-aggregate queries - should not skip check
		{
			name:        "Simple SELECT *",
			sql:         "SELECT * FROM users",
			wantHasAgg:  false,
			wantIsSelect: true,
		},
		{
			name:        "SELECT with columns",
			sql:         "SELECT id, name, email FROM users",
			wantHasAgg:  false,
			wantIsSelect: true,
		},
		{
			name:        "SELECT with WHERE",
			sql:         "SELECT * FROM users WHERE status = 'active'",
			wantHasAgg:  false,
			wantIsSelect: true,
		},
		{
			name:        "SELECT with JOIN",
			sql:         "SELECT u.name, o.amount FROM users u JOIN orders o ON u.id = o.user_id",
			wantHasAgg:  false,
			wantIsSelect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AnalyzeSQL(tt.sql)
			if err != nil {
				t.Fatalf("AnalyzeSQL() error = %v", err)
			}
			if result.HasTopAgg != tt.wantHasAgg {
				t.Errorf("HasTopAgg = %v, want %v", result.HasTopAgg, tt.wantHasAgg)
			}
			if result.IsSelectLike != tt.wantIsSelect {
				t.Errorf("IsSelectLike = %v, want %v", result.IsSelectLike, tt.wantIsSelect)
			}
		})
	}
}

func TestAnalyzeSQL_SubqueryWithAggregate(t *testing.T) {
	// Aggregate in subquery should NOT skip check for main query
	tests := []struct {
		name       string
		sql        string
		wantHasAgg bool
	}{
		{
			name:       "Aggregate in subquery only",
			sql:        "SELECT * FROM (SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id) t",
			wantHasAgg: false, // top-level has no aggregate
		},
		{
			name:       "Aggregate in WHERE subquery",
			sql:        "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders GROUP BY user_id HAVING COUNT(*) > 5)",
			wantHasAgg: false, // top-level has no aggregate
		},
		{
			name:       "Both top-level and subquery aggregates",
			sql:        "SELECT COUNT(*) FROM (SELECT user_id FROM orders GROUP BY user_id) t",
			wantHasAgg: true, // top-level has aggregate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AnalyzeSQL(tt.sql)
			if err != nil {
				t.Fatalf("AnalyzeSQL() error = %v", err)
			}
			if result.HasTopAgg != tt.wantHasAgg {
				t.Errorf("HasTopAgg = %v, want %v", result.HasTopAgg, tt.wantHasAgg)
			}
		})
	}
}

func TestAnalyzeSQL_LimitQueries(t *testing.T) {
	tests := []struct {
		name          string
		sql           string
		wantLimit     *int64
		wantIsSelect  bool
	}{
		{
			name:         "LIMIT 10",
			sql:          "SELECT * FROM users LIMIT 10",
			wantLimit:    ptr(int64(10)),
			wantIsSelect: true,
		},
		{
			name:         "LIMIT 100",
			sql:          "SELECT * FROM users LIMIT 100",
			wantLimit:    ptr(int64(100)),
			wantIsSelect: true,
		},
		{
			name:         "LIMIT 1000",
			sql:          "SELECT * FROM users LIMIT 1000",
			wantLimit:    ptr(int64(1000)),
			wantIsSelect: true,
		},
		{
			name:         "LIMIT with OFFSET",
			sql:          "SELECT * FROM users LIMIT 50 OFFSET 100",
			wantLimit:    ptr(int64(50)),
			wantIsSelect: true,
		},
		{
			name:         "No LIMIT",
			sql:          "SELECT * FROM users",
			wantLimit:    nil,
			wantIsSelect: true,
		},
		{
			name:         "LIMIT 0",
			sql:          "SELECT * FROM users LIMIT 0",
			wantLimit:    ptr(int64(0)),
			wantIsSelect: true,
		},
		{
			name:         "LIMIT 1",
			sql:          "SELECT * FROM users LIMIT 1",
			wantLimit:    ptr(int64(1)),
			wantIsSelect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AnalyzeSQL(tt.sql)
			if err != nil {
				t.Fatalf("AnalyzeSQL() error = %v", err)
			}
			if result.IsSelectLike != tt.wantIsSelect {
				t.Errorf("IsSelectLike = %v, want %v", result.IsSelectLike, tt.wantIsSelect)
			}
			if tt.wantLimit == nil {
				if result.LimitConst != nil {
					t.Errorf("LimitConst = %v, want nil", *result.LimitConst)
				}
			} else {
				if result.LimitConst == nil {
					t.Errorf("LimitConst = nil, want %v", *tt.wantLimit)
				} else if *result.LimitConst != *tt.wantLimit {
					t.Errorf("LimitConst = %v, want %v", *result.LimitConst, *tt.wantLimit)
				}
			}
		})
	}
}

func TestAnalyzeSQL_UnionQueries(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		wantHasAgg bool
		wantLimit  *int64
	}{
		{
			name:       "UNION without aggregate",
			sql:        "SELECT id, name FROM users UNION SELECT id, name FROM admins",
			wantHasAgg: false,
			wantLimit:  nil,
		},
		{
			name:       "UNION ALL without aggregate",
			sql:        "SELECT * FROM users UNION ALL SELECT * FROM admins",
			wantHasAgg: false,
			wantLimit:  nil,
		},
		{
			name:       "UNION with aggregate in all branches",
			sql:        "SELECT COUNT(*) FROM users UNION SELECT COUNT(*) FROM admins",
			wantHasAgg: true,
			wantLimit:  nil,
		},
		{
			name:       "UNION with aggregate in one branch only",
			sql:        "SELECT COUNT(*) FROM users UNION SELECT id FROM admins",
			wantHasAgg: false, // not all branches have aggregate
			wantLimit:  nil,
		},
		{
			name:       "UNION with outer LIMIT",
			sql:        "SELECT * FROM users UNION SELECT * FROM admins LIMIT 100",
			wantHasAgg: false,
			wantLimit:  ptr(int64(100)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AnalyzeSQL(tt.sql)
			if err != nil {
				t.Fatalf("AnalyzeSQL() error = %v", err)
			}
			if result.HasTopAgg != tt.wantHasAgg {
				t.Errorf("HasTopAgg = %v, want %v", result.HasTopAgg, tt.wantHasAgg)
			}
			if tt.wantLimit == nil {
				if result.LimitConst != nil {
					t.Errorf("LimitConst = %v, want nil", *result.LimitConst)
				}
			} else {
				if result.LimitConst == nil {
					t.Errorf("LimitConst = nil, want %v", *tt.wantLimit)
				} else if *result.LimitConst != *tt.wantLimit {
					t.Errorf("LimitConst = %v, want %v", *result.LimitConst, *tt.wantLimit)
				}
			}
		})
	}
}

func TestAnalyzeSQL_NonSelectStatements(t *testing.T) {
	tests := []struct {
		name         string
		sql          string
		wantIsSelect bool
	}{
		{
			name:         "SHOW DATABASES",
			sql:          "SHOW DATABASES",
			wantIsSelect: false,
		},
		{
			name:         "SHOW TABLES",
			sql:          "SHOW TABLES",
			wantIsSelect: false,
		},
		{
			name:         "DESCRIBE table",
			sql:          "DESCRIBE users",
			wantIsSelect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AnalyzeSQL(tt.sql)
			if err != nil {
				// Some statements may not be parseable, which is fine
				return
			}
			if result.IsSelectLike != tt.wantIsSelect {
				t.Errorf("IsSelectLike = %v, want %v", result.IsSelectLike, tt.wantIsSelect)
			}
		})
	}
}

func TestNeedsRowCountCheck(t *testing.T) {
	maxRows := 500

	tests := []struct {
		name         string
		sql          string
		wantNeedCheck bool
		wantReject   bool
	}{
		// Should skip check (needsCheck = false)
		{
			name:         "Aggregate COUNT(*)",
			sql:          "SELECT COUNT(*) FROM users",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "Aggregate SUM",
			sql:          "SELECT SUM(amount) FROM orders",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "Aggregate with GROUP BY",
			sql:          "SELECT user_id, COUNT(*) FROM orders GROUP BY user_id",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "LIMIT equal to max",
			sql:          "SELECT * FROM users LIMIT 500",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "LIMIT less than max",
			sql:          "SELECT * FROM users LIMIT 100",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "LIMIT 1",
			sql:          "SELECT * FROM users LIMIT 1",
			wantNeedCheck: false,
			wantReject:   false,
		},

		// LIMIT > maxRows still needs probe check (actual result might be smaller)
		{
			name:         "LIMIT exceeds max",
			sql:          "SELECT * FROM users LIMIT 1000",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "LIMIT much larger than max",
			sql:          "SELECT * FROM users LIMIT 10000",
			wantNeedCheck: true,
			wantReject:   false,
		},

		// Should execute probe check (needsCheck = true)
		{
			name:         "No LIMIT no aggregate",
			sql:          "SELECT * FROM users",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "SELECT with WHERE no LIMIT",
			sql:          "SELECT * FROM users WHERE status = 'active'",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "SELECT with JOIN no LIMIT",
			sql:          "SELECT u.*, o.* FROM users u JOIN orders o ON u.id = o.user_id",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "Aggregate in subquery only",
			sql:          "SELECT * FROM (SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id) t",
			wantNeedCheck: true,
			wantReject:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsCheck, directReject, _ := NeedsRowCountCheck(tt.sql, maxRows)
			if needsCheck != tt.wantNeedCheck {
				t.Errorf("needsCheck = %v, want %v", needsCheck, tt.wantNeedCheck)
			}
			if directReject != tt.wantReject {
				t.Errorf("directReject = %v, want %v", directReject, tt.wantReject)
			}
		})
	}
}

func TestNeedsRowCountCheck_DorisSpecificFunctions(t *testing.T) {
	maxRows := 500

	tests := []struct {
		name         string
		sql          string
		wantNeedCheck bool
	}{
		// Doris HLL functions
		{
			name:         "HLL_UNION_AGG",
			sql:          "SELECT HLL_UNION_AGG(hll_col) FROM user_stats",
			wantNeedCheck: false,
		},
		{
			name:         "HLL_CARDINALITY",
			sql:          "SELECT HLL_CARDINALITY(hll_col) FROM user_stats",
			wantNeedCheck: false,
		},
		// Doris Bitmap functions
		{
			name:         "BITMAP_UNION_COUNT",
			sql:          "SELECT BITMAP_UNION_COUNT(bitmap_col) FROM user_tags",
			wantNeedCheck: false,
		},
		{
			name:         "BITMAP_UNION",
			sql:          "SELECT BITMAP_UNION(bitmap_col) FROM user_tags GROUP BY category",
			wantNeedCheck: false,
		},
		// Other Doris aggregate functions
		{
			name:         "APPROX_COUNT_DISTINCT",
			sql:          "SELECT APPROX_COUNT_DISTINCT(user_id) FROM events",
			wantNeedCheck: false,
		},
		{
			name:         "GROUP_CONCAT",
			sql:          "SELECT GROUP_CONCAT(name) FROM users GROUP BY department",
			wantNeedCheck: false,
		},
		{
			name:         "PERCENTILE_APPROX",
			sql:          "SELECT PERCENTILE_APPROX(latency, 0.99) FROM requests",
			wantNeedCheck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsCheck, _, _ := NeedsRowCountCheck(tt.sql, maxRows)
			if needsCheck != tt.wantNeedCheck {
				t.Errorf("needsCheck = %v, want %v (should skip check for Doris aggregate functions)", needsCheck, tt.wantNeedCheck)
			}
		})
	}
}

func TestNeedsRowCountCheck_ComplexQueries(t *testing.T) {
	maxRows := 500

	tests := []struct {
		name         string
		sql          string
		wantNeedCheck bool
		wantReject   bool
	}{
		{
			name:         "CTE with aggregate",
			sql:          "WITH user_counts AS (SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id) SELECT * FROM user_counts",
			wantNeedCheck: true, // CTE result may have many rows
			wantReject:   false,
		},
		{
			name:         "Complex JOIN with aggregate",
			sql:          "SELECT u.department, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.department",
			wantNeedCheck: false, // has aggregate
			wantReject:   false,
		},
		{
			name:         "Nested subquery",
			sql:          "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders WHERE amount > 100)",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "DISTINCT query",
			sql:          "SELECT DISTINCT category FROM products",
			wantNeedCheck: true, // DISTINCT is not aggregate
			wantReject:   false,
		},
		{
			name:         "ORDER BY with LIMIT",
			sql:          "SELECT * FROM users ORDER BY created_at DESC LIMIT 100",
			wantNeedCheck: false, // has valid LIMIT
			wantReject:   false,
		},
		{
			name:         "Multiple aggregates in single query",
			sql:          "SELECT COUNT(*), SUM(amount), AVG(amount), MIN(amount), MAX(amount) FROM orders",
			wantNeedCheck: false,
			wantReject:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsCheck, directReject, _ := NeedsRowCountCheck(tt.sql, maxRows)
			if needsCheck != tt.wantNeedCheck {
				t.Errorf("needsCheck = %v, want %v", needsCheck, tt.wantNeedCheck)
			}
			if directReject != tt.wantReject {
				t.Errorf("directReject = %v, want %v", directReject, tt.wantReject)
			}
		})
	}
}

func TestNeedsRowCountCheck_EdgeCases(t *testing.T) {
	maxRows := 500

	tests := []struct {
		name         string
		sql          string
		wantNeedCheck bool
		wantReject   bool
	}{
		{
			name:         "Empty-ish LIMIT 0",
			sql:          "SELECT * FROM users LIMIT 0",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "LIMIT at boundary",
			sql:          "SELECT * FROM users LIMIT 501",
			wantNeedCheck: true, // 501 > 500, needs probe check
			wantReject:   false,
		},
		{
			name:         "SELECT with trailing semicolon",
			sql:          "SELECT * FROM users;",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "SELECT with extra whitespace",
			sql:          "  SELECT * FROM users  ",
			wantNeedCheck: true,
			wantReject:   false,
		},
		{
			name:         "Lowercase keywords",
			sql:          "select count(*) from users",
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "Mixed case keywords",
			sql:          "Select Count(*) From users",
			wantNeedCheck: false,
			wantReject:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsCheck, directReject, _ := NeedsRowCountCheck(tt.sql, maxRows)
			if needsCheck != tt.wantNeedCheck {
				t.Errorf("needsCheck = %v, want %v", needsCheck, tt.wantNeedCheck)
			}
			if directReject != tt.wantReject {
				t.Errorf("directReject = %v, want %v", directReject, tt.wantReject)
			}
		})
	}
}

func TestNeedsRowCountCheck_DifferentMaxRows(t *testing.T) {
	tests := []struct {
		name         string
		sql          string
		maxRows      int
		wantNeedCheck bool
		wantReject   bool
	}{
		{
			name:         "LIMIT 100 with maxRows 50",
			sql:          "SELECT * FROM users LIMIT 100",
			maxRows:      50,
			wantNeedCheck: true, // LIMIT > maxRows, needs probe check
			wantReject:   false,
		},
		{
			name:         "LIMIT 100 with maxRows 100",
			sql:          "SELECT * FROM users LIMIT 100",
			maxRows:      100,
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "LIMIT 100 with maxRows 200",
			sql:          "SELECT * FROM users LIMIT 100",
			maxRows:      200,
			wantNeedCheck: false,
			wantReject:   false,
		},
		{
			name:         "No LIMIT with maxRows 1000",
			sql:          "SELECT * FROM users",
			maxRows:      1000,
			wantNeedCheck: true,
			wantReject:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsCheck, directReject, _ := NeedsRowCountCheck(tt.sql, tt.maxRows)
			if needsCheck != tt.wantNeedCheck {
				t.Errorf("needsCheck = %v, want %v", needsCheck, tt.wantNeedCheck)
			}
			if directReject != tt.wantReject {
				t.Errorf("directReject = %v, want %v", directReject, tt.wantReject)
			}
		})
	}
}

// TestSummary_SkipProbeCheck prints a summary of which SQL patterns skip the probe check
func TestSummary_SkipProbeCheck(t *testing.T) {
	maxRows := 500

	skipCheckCases := []struct {
		category string
		sql      string
	}{
		// Aggregate functions
		{"Aggregate - COUNT(*)", "SELECT COUNT(*) FROM users"},
		{"Aggregate - COUNT(col)", "SELECT COUNT(id) FROM users"},
		{"Aggregate - SUM", "SELECT SUM(amount) FROM orders"},
		{"Aggregate - AVG", "SELECT AVG(price) FROM products"},
		{"Aggregate - MIN", "SELECT MIN(created_at) FROM logs"},
		{"Aggregate - MAX", "SELECT MAX(score) FROM results"},
		{"Aggregate - GROUP BY", "SELECT user_id, COUNT(*) FROM orders GROUP BY user_id"},
		{"Aggregate - HAVING", "SELECT user_id, SUM(amount) FROM orders GROUP BY user_id HAVING SUM(amount) > 1000"},

		// Doris specific aggregates
		{"Doris - HLL_UNION_AGG", "SELECT HLL_UNION_AGG(hll_col) FROM stats"},
		{"Doris - BITMAP_UNION_COUNT", "SELECT BITMAP_UNION_COUNT(bitmap_col) FROM tags"},
		{"Doris - APPROX_COUNT_DISTINCT", "SELECT APPROX_COUNT_DISTINCT(user_id) FROM events"},
		{"Doris - GROUP_CONCAT", "SELECT GROUP_CONCAT(name) FROM users GROUP BY dept"},

		// LIMIT <= maxRows
		{"LIMIT - Equal to max", "SELECT * FROM users LIMIT 500"},
		{"LIMIT - Less than max", "SELECT * FROM users LIMIT 100"},
		{"LIMIT - With OFFSET", "SELECT * FROM users LIMIT 100 OFFSET 50"},
		{"LIMIT - Value 1", "SELECT * FROM users LIMIT 1"},
		{"LIMIT - Value 0", "SELECT * FROM users LIMIT 0"},
	}

	t.Log("=== SQL patterns that SKIP probe check (no extra query needed) ===")
	for _, tc := range skipCheckCases {
		needsCheck, _, _ := NeedsRowCountCheck(tc.sql, maxRows)
		status := "✓ SKIP"
		if needsCheck {
			status = "✗ NEEDS CHECK (unexpected)"
		}
		t.Logf("  %s: %s\n    SQL: %s", status, tc.category, tc.sql)
	}

	needsCheckCases := []struct {
		category string
		sql      string
	}{
		{"No LIMIT - Simple SELECT", "SELECT * FROM users"},
		{"No LIMIT - With WHERE", "SELECT * FROM users WHERE status = 'active'"},
		{"No LIMIT - With JOIN", "SELECT u.*, o.* FROM users u JOIN orders o ON u.id = o.user_id"},
		{"No LIMIT - Subquery with agg", "SELECT * FROM (SELECT user_id, COUNT(*) FROM orders GROUP BY user_id) t"},
		{"No LIMIT - DISTINCT", "SELECT DISTINCT category FROM products"},
		{"LIMIT > max (actual may be smaller)", "SELECT * FROM users LIMIT 1000"},
		{"LIMIT >> max", "SELECT * FROM users LIMIT 10000"},
	}

	t.Log("\n=== SQL patterns that NEED probe check ===")
	for _, tc := range needsCheckCases {
		needsCheck, _, _ := NeedsRowCountCheck(tc.sql, maxRows)
		status := "✓ NEEDS CHECK"
		if !needsCheck {
			status = "✗ SKIP (unexpected)"
		}
		t.Logf("  %s: %s\n    SQL: %s", status, tc.category, tc.sql)
	}
}

// ptr is a helper function to create a pointer to int64
func ptr(v int64) *int64 {
	return &v
}
