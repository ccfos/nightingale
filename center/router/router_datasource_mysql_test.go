package router

import (
	"strings"
	"testing"
)

func TestMysqlCheckExtraParams(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "timeout=10s"},
		{"plain", "charset=utf8mb4", "charset=utf8mb4&timeout=10s"},
		{"leading question mark", "?tls=true", "tls=true&timeout=10s"},
		{"user timeout kept", "timeout=30s", "timeout=30s"},
		{"user timeout among others", "tls=true&timeout=30s", "tls=true&timeout=30s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mysqlCheckExtraParams(tc.in); got != tc.want {
				t.Errorf("mysqlCheckExtraParams(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCheckMysqlDatasource(t *testing.T) {
	t.Run("real connection attempt to closed port", func(t *testing.T) {
		// Point at a closed local port: if the code never dials for real, this
		// case would not fail as expected, proving checkMysqlDatasource performs
		// an actual TCP/MySQL connection attempt.
		settings := map[string]interface{}{
			"mysql.shards": []interface{}{
				map[string]interface{}{
					"mysql.addr":     "127.0.0.1:1",
					"mysql.user":     "root",
					"mysql.password": "root",
				},
			},
		}
		err := checkMysqlDatasource(settings)
		if err == nil {
			t.Fatal("expected error for unreachable mysql addr, got nil")
		}
		if !strings.Contains(err.Error(), "mysql connect failed") {
			t.Errorf("expected error to mention connect stage, got: %v", err)
		}
	})

	t.Run("empty settings", func(t *testing.T) {
		if err := checkMysqlDatasource(nil); err == nil {
			t.Fatal("expected error for empty settings, got nil")
		}
	})

	t.Run("missing addr", func(t *testing.T) {
		settings := map[string]interface{}{
			"mysql.shards": []interface{}{
				map[string]interface{}{"mysql.user": "root"},
			},
		}
		if err := checkMysqlDatasource(settings); err == nil {
			t.Fatal("expected error for missing addr, got nil")
		}
	})

	t.Run("missing user", func(t *testing.T) {
		settings := map[string]interface{}{
			"mysql.shards": []interface{}{
				map[string]interface{}{"mysql.addr": "127.0.0.1:3306"},
			},
		}
		if err := checkMysqlDatasource(settings); err == nil {
			t.Fatal("expected error for missing user, got nil")
		}
	})
}
