package iotdb

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestRPCIntegration is opt-in because it requires a reachable IoTDB instance
// and credentials. Keep credentials in the process environment only; never put
// them in the repository or test output.
func TestRPCIntegration(t *testing.T) {
	if os.Getenv("IOTDB_INTEGRATION") != "1" {
		t.Skip("set IOTDB_INTEGRATION=1 to run the live IoTDB RPC test")
	}

	addr := strings.TrimSpace(os.Getenv("IOTDB_RPC_ADDR"))
	user := os.Getenv("IOTDB_USER")
	password := os.Getenv("IOTDB_PASSWORD")
	database := strings.TrimSpace(os.Getenv("IOTDB_DATABASE"))
	if addr == "" || user == "" || password == "" || database == "" {
		t.Skip("IOTDB_RPC_ADDR, IOTDB_USER, IOTDB_PASSWORD and IOTDB_DATABASE are required")
	}

	it := &Iotdb{
		RPCAddr:     addr,
		Database:    database,
		Basic:       &IotdbBasicAuth{User: user, Password: password},
		DialTimeout: 5000,
		Timeout:     10000,
	}
	t.Cleanup(func() { _ = it.Close() })

	ctx := context.Background()
	if err := it.TestRPC(ctx); err != nil {
		t.Fatalf("IoTDB RPC health query failed: %v", err)
	}

	table := strings.TrimSpace(os.Getenv("IOTDB_TEST_TABLE"))
	if table == "" {
		table = "active_loading_files_number_total"
	}
	valueColumn := strings.TrimSpace(os.Getenv("IOTDB_TEST_VALUE_COLUMN"))
	if valueColumn == "" {
		valueColumn = table
	}
	resp, err := it.QueryTable(ctx, database,
		"SELECT time, instance, "+valueColumn+" FROM "+table+" LIMIT 3", 3)
	if err != nil {
		t.Fatalf("IoTDB RPC table query failed: %v", err)
	}
	if len(resp.ColumnNames) != 3 {
		t.Fatalf("unexpected columns: %#v", resp.ColumnNames)
	}
	if len(resp.Values) == 0 || len(resp.Values) > 3 {
		t.Fatalf("unexpected row count: %d", len(resp.Values))
	}
}
