package tdengine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newRecordingClient returns a client whose /rest/sql endpoint records every
// statement it receives and answers with an empty result set.
func newRecordingClient(t *testing.T) (*Tdengine, *[]string) {
	t.Helper()
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		w.Write([]byte(`{"code":0,"column_meta":[],"data":[],"rows":0}`))
	}))
	t.Cleanup(srv.Close)

	tc := &Tdengine{Addr: srv.URL}
	tc.InitCli()
	return tc, &got
}

func TestShowTablesRejectsNonDatabaseInput(t *testing.T) {
	tc, got := newRecordingClient(t)
	ctx := context.Background()

	for _, in := range []string{
		"",
		"secretdb.tables",
		"db; drop database db",
		"db tables",
		"`db`",
		"1db",
	} {
		if _, err := tc.ShowTables(ctx, in); err == nil {
			t.Errorf("ShowTables(%q) expected error", in)
		}
		if _, err := tc.ShowSTables(ctx, in); err == nil {
			t.Errorf("ShowSTables(%q) expected error", in)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("invalid input reached TDengine: %q", *got)
	}

	// "users" is a valid identifier; it must be treated as a database name and
	// pinned to "<db>.TABLES" instead of becoming "SHOW users".
	if _, err := tc.ShowTables(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.ShowSTables(ctx, "power_db"); err != nil {
		t.Fatal(err)
	}
	want := []string{"SHOW users.TABLES", "SHOW power_db.STABLES"}
	if len(*got) != len(want) || (*got)[0] != want[0] || (*got)[1] != want[1] {
		t.Fatalf("got %q, want %q", *got, want)
	}
}

func TestDescribeTableIdentifiers(t *testing.T) {
	tc, got := newRecordingClient(t)
	ctx := context.Background()

	bad := []map[string]string{
		{"database": "information_schema.ins_users", "table": "t"},
		{"database": "db", "table": "other.t"},
		{"database": "db", "table": "t`; drop"},
		{"database": "db", "table": "t\nx"},
		{"database": "db", "table": ""},
	}
	for _, q := range bad {
		if _, err := tc.DescribeTable(ctx, q); err == nil {
			t.Errorf("DescribeTable(%v) expected error", q)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("invalid input reached TDengine: %q", *got)
	}

	if _, err := tc.DescribeTable(ctx, map[string]string{"database": "db", "table": "meters"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.DescribeTable(ctx, map[string]string{"database": "db", "table": "cpu-usage"}); err != nil {
		t.Fatal(err)
	}
	// Spaces are legal in escaped TDengine names; backticks keep the whole
	// value a single identifier, so nothing after it is parsed as SQL.
	if _, err := tc.DescribeTable(ctx, map[string]string{"database": "db", "table": "t union select * from x"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"select * from db.meters limit 1",
		"select * from db.`cpu-usage` limit 1",
		"select * from db.`t union select * from x` limit 1",
	}
	if len(*got) != len(want) || (*got)[0] != want[0] || (*got)[1] != want[1] || (*got)[2] != want[2] {
		t.Fatalf("got %q, want %q", *got, want)
	}
}
