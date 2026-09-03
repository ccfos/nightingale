package ormx

import (
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestParseMysqlDatabaseDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		dbName   string
		net      string
		addr     string
		paramKey string
		paramVal string
	}{
		{
			name:   "TCP",
			dsn:    "root:1234@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local&allowNativePasswords=true",
			dbName: "test",
			net:    "tcp",
			addr:   "127.0.0.1:3306",
		},
		{
			name:   "TCP non-default port",
			dsn:    "root:1234@tcp(127.0.0.1:3307)/test?charset=utf8mb4&parseTime=True&loc=Local&allowNativePasswords=true",
			dbName: "test",
			net:    "tcp",
			addr:   "127.0.0.1:3307",
		},
		{
			name:   "Unix socket",
			dsn:    "root:1234@unix(/var/run/mysqld/mysqld.sock)/test?charset=utf8mb4&parseTime=True&loc=Local&allowNativePasswords=true",
			dbName: "test",
			net:    "unix",
			addr:   "/var/run/mysqld/mysqld.sock",
		},
		{
			name:     "Unix socket with escaped slash in query param",
			dsn:      "root:1234@unix(/var/run/mysqld/mysqld.sock)/test?charset=utf8mb4&arg=%2Fsome%2Fpath.ext",
			dbName:   "test",
			net:      "unix",
			addr:     "/var/run/mysqld/mysqld.sock",
			paramKey: "arg",
			paramVal: "/some/path.ext",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbName, serverDSN, err := parseMysqlDatabaseDSN(tt.dsn)
			require.NoError(t, err)
			require.Equal(t, tt.dbName, dbName)

			cfg, err := mysqlDriver.ParseDSN(serverDSN)
			require.NoError(t, err)
			require.Empty(t, cfg.DBName)
			require.Equal(t, tt.net, cfg.Net)
			require.Equal(t, tt.addr, cfg.Addr)
			if tt.paramKey != "" {
				require.Equal(t, tt.paramVal, cfg.Params[tt.paramKey])
			}
		})
	}
}

func TestParseMysqlDatabaseDSNWithoutDBName(t *testing.T) {
	_, _, err := parseMysqlDatabaseDSN("root:1234@tcp(127.0.0.1:3306)/?charset=utf8mb4")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse database name from DSN")
}
