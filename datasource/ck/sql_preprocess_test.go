package ck

import (
	"testing"
)

func TestQueryDataWithoutMacrosSkipsPreprocess(t *testing.T) {
	origPreprocess := SQLPreprocess
	SQLPreprocess = func(sql string, from, to int64) (string, error) {
		t.Fatalf("SQLPreprocess should not be called for SQL without macros")
		return sql, nil
	}
	defer func() { SQLPreprocess = origPreprocess }()

	c := &Clickhouse{}
	_, err := c.QueryData(t.Context(), map[string]interface{}{
		"sql":      "SELECT 1",
		"from":     int64(0),
		"to":       int64(60),
		"keys":     map[string]interface{}{"valueKey": "v"},
		"database": "default",
	})
	if err == nil {
		t.Fatal("expected error from missing client, got nil")
	}
}

func TestDefaultSQLPreprocessDelegatesToMacro(t *testing.T) {
	origMacro := SQLPreprocess
	defer func() { SQLPreprocess = origMacro }()

	called := false
	SQLPreprocess = func(sql string, from, to int64) (string, error) {
		called = true
		return sql, nil
	}

	preprocess := SQLPreprocess
	if preprocess == nil {
		preprocess = defaultSQLPreprocess
	}
	_, err := preprocess("SELECT $__timeFilter(ts)", 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected custom SQLPreprocess to be called")
	}
}
