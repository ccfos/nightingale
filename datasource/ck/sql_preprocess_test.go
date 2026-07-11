package ck

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/macros"
)

func TestQueryDataPassesClickHouseDatasourceTypeToMacro(t *testing.T) {
	origMacro := macros.Macro
	t.Cleanup(func() { macros.Macro = origMacro })

	var gotType macros.DatasourceType
	macros.RegisterMacro(func(sql string, from, to int64, datasourceType macros.DatasourceType) (string, error) {
		gotType = datasourceType
		return sql, nil
	})

	c := &Clickhouse{}
	_, err := c.QueryData(t.Context(), map[string]interface{}{
		"sql":      "SELECT $__timeFilter(ts)",
		"from":     int64(0),
		"to":       int64(60),
		"keys":     map[string]interface{}{"valueKey": "v"},
		"database": "default",
	})
	if err == nil {
		t.Fatal("expected error from missing client, got nil")
	}
	if gotType != macros.DatasourceTypeClickHouse {
		t.Fatalf("got datasource type %q, want %q", gotType, macros.DatasourceTypeClickHouse)
	}
}

func TestQueryLogPassesClickHouseDatasourceTypeToMacro(t *testing.T) {
	origMacro := macros.Macro
	t.Cleanup(func() { macros.Macro = origMacro })

	var gotType macros.DatasourceType
	macros.RegisterMacro(func(sql string, from, to int64, datasourceType macros.DatasourceType) (string, error) {
		gotType = datasourceType
		return sql, nil
	})

	c := &Clickhouse{}
	_, _, err := c.QueryLog(t.Context(), map[string]interface{}{
		"sql":      "SELECT $__timeFilter(ts)",
		"from":     int64(0),
		"to":       int64(60),
		"database": "default",
	})
	if err == nil {
		t.Fatal("expected error from missing client, got nil")
	}
	if gotType != macros.DatasourceTypeClickHouse {
		t.Fatalf("got datasource type %q, want %q", gotType, macros.DatasourceTypeClickHouse)
	}
}

func TestQueryDataWithoutMacrosSkipsMacro(t *testing.T) {
	origMacro := macros.Macro
	t.Cleanup(func() { macros.Macro = origMacro })
	macros.RegisterMacro(func(sql string, from, to int64, datasourceType macros.DatasourceType) (string, error) {
		t.Fatal("Macro should not be called for SQL without macros")
		return sql, nil
	})

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
