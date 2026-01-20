package doris

import (
	"strings"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver" // required for parser
)

// SQLAnalyzeResult holds the analysis result of a SQL statement
type SQLAnalyzeResult struct {
	IsSelectLike bool   // whether the statement is a SELECT-like query
	HasTopAgg    bool   // whether the top-level query has aggregate functions
	LimitConst   *int64 // top-level LIMIT constant value (nil if no LIMIT or non-constant)
}

// AnalyzeSQL analyzes a SQL statement and extracts top-level features
func AnalyzeSQL(sql string) (*SQLAnalyzeResult, error) {
	p := parser.New()
	stmtNodes, _, err := p.Parse(sql, "", "")
	if err != nil {
		return nil, err
	}
	if len(stmtNodes) == 0 {
		return &SQLAnalyzeResult{}, nil
	}

	result := &SQLAnalyzeResult{}
	stmt := stmtNodes[0]

	switch s := stmt.(type) {
	case *ast.SelectStmt:
		result.IsSelectLike = true
		analyzeSelectStmt(s, result)
	case *ast.SetOprStmt: // UNION / INTERSECT / EXCEPT
		result.IsSelectLike = true
		analyzeSetOprStmt(s, result)
	default:
		result.IsSelectLike = false
	}

	return result, nil
}

// analyzeSelectStmt analyzes a SELECT statement
func analyzeSelectStmt(sel *ast.SelectStmt, result *SQLAnalyzeResult) {
	// Check if top-level SELECT has aggregate functions
	if sel.Fields != nil {
		for _, field := range sel.Fields.Fields {
			if field.Expr != nil && hasAggregateFunc(field.Expr) {
				result.HasTopAgg = true
				break
			}
		}
	}

	// Extract top-level LIMIT
	if sel.Limit != nil && sel.Limit.Count != nil {
		if val, ok := extractConstValue(sel.Limit.Count); ok {
			result.LimitConst = &val
		}
	}
}

// analyzeSetOprStmt analyzes UNION/INTERSECT/EXCEPT statements
func analyzeSetOprStmt(setOpr *ast.SetOprStmt, result *SQLAnalyzeResult) {
	// UNION's LIMIT is at the outermost level
	if setOpr.Limit != nil && setOpr.Limit.Count != nil {
		if val, ok := extractConstValue(setOpr.Limit.Count); ok {
			result.LimitConst = &val
		}
	}

	// Check if all branches are aggregates (conservative: if any is non-aggregate, don't skip)
	if setOpr.SelectList == nil || len(setOpr.SelectList.Selects) == 0 {
		return
	}

	allAgg := true
	for _, sel := range setOpr.SelectList.Selects {
		if selectStmt, ok := sel.(*ast.SelectStmt); ok {
			if selectStmt.Fields != nil {
				hasAgg := false
				for _, field := range selectStmt.Fields.Fields {
					if field.Expr != nil && hasAggregateFunc(field.Expr) {
						hasAgg = true
						break
					}
				}
				if !hasAgg {
					allAgg = false
					break
				}
			}
		}
	}
	result.HasTopAgg = allAgg
}

// hasAggregateFunc checks if an expression contains aggregate functions (without entering subqueries)
func hasAggregateFunc(expr ast.ExprNode) bool {
	checker := &aggregateChecker{}
	expr.Accept(checker)
	return checker.found
}

// aggregateChecker implements ast.Visitor to find aggregate functions
type aggregateChecker struct {
	found bool
}

func (c *aggregateChecker) Enter(n ast.Node) (ast.Node, bool) {
	if c.found {
		return n, true // stop traversal
	}

	switch node := n.(type) {
	case *ast.SubqueryExpr:
		return n, true // don't enter subquery
	case *ast.AggregateFuncExpr:
		c.found = true
		return n, true
	case *ast.FuncCallExpr:
		// Check for Doris-specific aggregate/statistic functions
		funcName := strings.ToUpper(node.FnName.L)
		if isDorisAggregateFunc(funcName) {
			c.found = true
			return n, true
		}
	}
	return n, false // continue traversal
}

func (c *aggregateChecker) Leave(n ast.Node) (ast.Node, bool) {
	return n, true
}

// isDorisAggregateFunc checks if a function is a Doris-specific aggregate/statistic function
func isDorisAggregateFunc(funcName string) bool {
	dorisAggFuncs := map[string]bool{
		// Standard aggregates (in case parser doesn't recognize them)
		"COUNT":     true,
		"SUM":       true,
		"AVG":       true,
		"MIN":       true,
		"MAX":       true,
		"ANY":       true,
		"ANY_VALUE": true,

		// HLL related
		"HLL_UNION_AGG":   true,
		"HLL_RAW_AGG":     true,
		"HLL_CARDINALITY": true,
		"HLL_UNION":       true,
		"HLL_HASH":        true,

		// Bitmap related
		"BITMAP_UNION":         true,
		"BITMAP_UNION_COUNT":   true,
		"BITMAP_INTERSECT":     true,
		"BITMAP_COUNT":         true,
		"BITMAP_AND_COUNT":     true,
		"BITMAP_OR_COUNT":      true,
		"BITMAP_XOR_COUNT":     true,
		"BITMAP_AND_NOT_COUNT": true,

		// Other aggregates
		"PERCENTILE":            true,
		"PERCENTILE_APPROX":     true,
		"APPROX_COUNT_DISTINCT": true,
		"NDV":                   true,
		"COLLECT_LIST":          true,
		"COLLECT_SET":           true,
		"GROUP_CONCAT":          true,
		"GROUP_BIT_AND":         true,
		"GROUP_BIT_OR":          true,
		"GROUP_BIT_XOR":         true,
		"GROUPING":              true,
		"GROUPING_ID":           true,

		// Statistical functions
		"STDDEV":      true,
		"STDDEV_POP":  true,
		"STDDEV_SAMP": true,
		"STD":         true,
		"VARIANCE":    true,
		"VAR_POP":     true,
		"VAR_SAMP":    true,
		"COVAR_POP":   true,
		"COVAR_SAMP":  true,
		"CORR":        true,

		// Window functions that are also aggregates
		"FIRST_VALUE":  true,
		"LAST_VALUE":   true,
		"LAG":          true,
		"LEAD":         true,
		"ROW_NUMBER":   true,
		"RANK":         true,
		"DENSE_RANK":   true,
		"NTILE":        true,
		"CUME_DIST":    true,
		"PERCENT_RANK": true,
	}
	return dorisAggFuncs[funcName]
}

// extractConstValue extracts constant integer value from an expression
func extractConstValue(expr ast.ExprNode) (int64, bool) {
	switch v := expr.(type) {
	case ast.ValueExpr:
		switch val := v.GetValue().(type) {
		case int64:
			return val, true
		case uint64:
			return int64(val), true
		case float64:
			return int64(val), true
		case int:
			return int64(val), true
		}
	}
	return 0, false
}

// NeedsRowCountCheck determines if a SQL query needs row count checking
// Returns: needsCheck bool, directReject bool, rejectReason string
func NeedsRowCountCheck(sql string, maxQueryRows int) (bool, bool, string) {
	result, err := AnalyzeSQL(sql)
	if err != nil {
		// Parse failed, fall back to probe check
		return true, false, ""
	}

	if !result.IsSelectLike {
		// Not a SELECT query, skip check
		return false, false, ""
	}

	// Rule 1: Top-level has aggregate functions -> skip check
	if result.HasTopAgg {
		return false, false, ""
	}

	// Rule 2: Top-level LIMIT <= maxRows -> skip check
	if result.LimitConst != nil && *result.LimitConst <= int64(maxQueryRows) {
		return false, false, ""
	}

	// Otherwise, needs probe check (including LIMIT > maxRows, since actual result may be smaller)
	return true, false, ""
}
