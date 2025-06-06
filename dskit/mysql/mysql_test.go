// @Author: Ciusyan 5/11/24

package mysql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewMySQLWithSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings interface{}
		wantErr  bool
	}{
		{
			name:     "valid string settings",
			settings: `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`,
			wantErr:  false,
		},
		{
			name:     "invalid settings type",
			settings: 12345,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMySQLWithSettings(context.Background(), tt.settings)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMySQLWithSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
			t.Log(got)
		})
	}
}

func TestNewConn(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	tests := []struct {
		name     string
		database string
		wantErr  bool
	}{
		{
			name:     "valid connection",
			database: "db1",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mysql.NewConn(ctx, tt.database)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewConn() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestShowDatabases(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	databases, err := mysql.ShowDatabases(ctx)
	require.NoError(t, err)
	t.Log(databases)
}

func TestShowTables(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	tables, err := mysql.ShowTables(ctx, "db1")
	require.NoError(t, err)
	t.Log(tables)
}

func TestDescTable(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	descTable, err := mysql.DescTable(ctx, "db1", "students")
	require.NoError(t, err)
	for _, desc := range descTable {
		t.Logf("%+v", *desc)
	}
}

func TestExecQuery(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	rows, err := mysql.ExecQuery(ctx, "db1", "SELECT * FROM students WHERE id = 10008")
	require.NoError(t, err)
	for _, row := range rows {
		t.Log(row)
	}
}

func TestSelectRows(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	rows, err := mysql.SelectRows(ctx, "db1", "students", "id > 10008")
	require.NoError(t, err)
	for _, row := range rows {
		t.Log(row)
	}
}
