package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/dskit/mysql"
)

// mysqlCheckDialTimeout is the dial timeout injected for the save-time
// connectivity check. MySQL has no HTTP endpoint to probe, so the check has to
// open a real TCP connection; a plain DSN carries no timeout, and an unreachable
// host would otherwise block until the OS-level connect timeout (tens of seconds
// to minutes). This bounds it to match the DatasourceCheck budget (10s). It only
// applies to the test connection, never to the settings persisted to the DB.
const mysqlCheckDialTimeout = "timeout=10s"

// checkMysqlDatasource runs a real connectivity check against a MySQL datasource,
// mirroring the save-time check used for clickhouse: it decodes the mysql.shards
// settings, opens a connection (NewConn pings internally) and runs SHOW DATABASES.
// On failure it returns a descriptive error that the caller decides whether to
// surface as a save blocker.
func checkMysqlDatasource(settings map[string]interface{}) error {
	if len(settings) == 0 {
		return fmt.Errorf("mysql settings is empty, please check datasource setting")
	}

	m, err := mysql.NewMySQLWithSettings(context.Background(), settings)
	if err != nil {
		return fmt.Errorf("parse mysql settings failed: %v", err)
	}

	if len(m.Shards) == 0 || len(strings.TrimSpace(m.Shards[0].Addr)) == 0 {
		return fmt.Errorf("mysql addr is invalid, please check datasource setting")
	}
	if len(strings.TrimSpace(m.Shards[0].User)) == 0 {
		return fmt.Errorf("mysql user is invalid, please check datasource setting")
	}

	// Deep-copy the config for the check only: the dial timeout must not leak
	// into the user's persisted dsn_extra_params.
	cfg := *m
	cfg.Shards = append([]mysql.Shard(nil), m.Shards...)
	cfg.Shards[0].DsnExtraParams = mysqlCheckExtraParams(cfg.Shards[0].DsnExtraParams)

	if _, err := cfg.NewConn(context.Background(), ""); err != nil {
		return fmt.Errorf("mysql connect failed: %v", err)
	}
	if _, err := cfg.ShowDatabases(context.Background()); err != nil {
		return fmt.Errorf("mysql query test failed: %v", err)
	}
	return nil
}

// mysqlCheckExtraParams appends a dial timeout to the user's dsn_extra_params;
// if the user already configured a timeout explicitly, theirs is kept.
func mysqlCheckExtraParams(extra string) string {
	extra = strings.TrimSpace(extra)
	extra = strings.TrimPrefix(extra, "?")
	if strings.Contains(extra, "timeout=") {
		return extra
	}
	if extra == "" {
		return mysqlCheckDialTimeout
	}
	return extra + "&" + mysqlCheckDialTimeout
}
