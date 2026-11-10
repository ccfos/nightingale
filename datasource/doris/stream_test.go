package doris

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	kit "github.com/ccfos/nightingale/v6/dskit/doris"
	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/pkg/macros"
	_ "github.com/mattn/go-sqlite3"
)

func TestQueryLogStreamCompatibility(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	address := t.Name()
	pool.PoolClient.Store(address+"::", db)
	t.Cleanup(func() { pool.PoolClient.Delete(address + "::"); db.Close() })
	client := &Doris{Doris: kit.Doris{Addr: address, Timeout: 5000, MaxIdleConns: 1, MaxOpenConns: 1, ConnMaxLifetime: 60, MaxQueryRows: 2}}
	previous := macros.Macro
	defer func() { macros.Macro = previous }()
	macros.Macro = func(q string, from, to int64, cate string) (string, error) {
		if from != 990 || to != 1990 || cate != DorisType {
			t.Fatalf("macro args=%d,%d,%s", from, to, cate)
		}
		return strings.ReplaceAll(q, "$__timeFrom()", "990"), nil
	}
	for _, tc := range []struct {
		name, sql       string
		readonly, valid bool
	}{
		{"types", "SELECT 1 AS n, 'text' AS message, NULL AS missing LIMIT 1", false, true},
		{"empty", "SELECT 1 AS n WHERE 0 LIMIT 1", false, true},
		{"macro", "SELECT $__timeFrom() AS n LIMIT 1", false, true},
		{"banned", "CREATE TABLE forbidden (id INT)", false, false},
		{"readonly", "DELETE FROM forbidden LIMIT 1", true, false},
		{"probe rejects", "SELECT 1 AS n UNION ALL SELECT 2 UNION ALL SELECT 3", false, false},
		{"sql error", "SELECT * FROM missing LIMIT 1", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			param := &QueryParam{SQL: tc.sql, From: 1000, To: 2000, Offset: 10}
			ctx := types.WithCallContext(context.Background(), types.CallContext{EnforceReadOnly: tc.readonly})
			collected, total, oldErr := client.QueryLog(ctx, param)
			streamed := make([]any, 0)
			n, err := client.QueryLogStream(ctx, param, 100, func(row map[string]any) error { streamed = append(streamed, row); return nil })
			if tc.valid {
				if oldErr != nil || err != nil || n != total || !reflect.DeepEqual(collected, streamed) {
					t.Fatalf("old=%v/%d/%v stream=%v/%d/%v", collected, total, oldErr, streamed, n, err)
				}
			} else if err == nil || oldErr == nil {
				t.Fatalf("old=%v stream=%v", oldErr, err)
			}
			if param.SQL != tc.sql || param.From != 1000 || param.To != 2000 || param.Offset != 10 {
				t.Fatalf("mutated param=%+v", param)
			}
		})
	}
	expected := errors.New("writer stopped")
	n, err := client.QueryLogStream(context.Background(), &QueryParam{SQL: "SELECT 1 AS n UNION ALL SELECT 2 LIMIT 2"}, 100, func(map[string]any) error { return expected })
	if n != 0 || !errors.Is(err, expected) {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
