// @Author: Ciusyan 5/17/24

package postgres

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	ctx := context.Background()
	settings := `{"pgsql.addr":"example.aliyuncs.com:80","pgsql.db":"hg_test","pgsql.user":"LTAIxxxxxxxxxxxxxxxx","pgsql.password":"xxxxxxxxxxxxxxxxxxxxxxxxxx","pgsql.maxIdleConns":5,"pgsql.maxOpenConns":10,"pgsql.connMaxLifetime":30}`
	postgres, err := NewPostgreSQLWithSettings(ctx, settings)
	require.NoError(t, err)

	param := &sqlbase.QueryParam{
		Sql: "SELECT * FROM flashcat_test WHERE CAST(id AS INTEGER) > 0",
		Keys: types.Keys{
			ValueKey:   "",
			LabelKey:   "",
			TimeKey:    "",
			TimeFormat: "",
		},
	}

	rows, err := postgres.Query(ctx, param)
	require.NoError(t, err)
	for _, row := range rows {
		t.Log(row)
	}
}

func TestQueryTimeseries(t *testing.T) {
	ctx := context.Background()
	settings := `{"pgsql.addr":"example.aliyuncs.com:80","pgsql.db":"hg_test","pgsql.user":"LTAIxxxxxxxxxxxxxxxx","pgsql.password":"xxxxxxxxxxxxxxxxxxxxxxxxxx","pgsql.maxIdleConns":5,"pgsql.maxOpenConns":10,"pgsql.connMaxLifetime":30}`
	postgres, err := NewPostgreSQLWithSettings(ctx, settings)
	require.NoError(t, err)

	// Prepare a test query parameter
	param := &sqlbase.QueryParam{
		Sql: "SELECT id as aid, age as my_age FROM flashcat_test",
		Keys: types.Keys{
			ValueKey:   "my_age aid",                   // Set the value key to the column name containing the metric value
			LabelKey:   "addr",                         // Set the label key to the column name containing the metric label
			TimeKey:    "create_time",                  // Set the time key to the column name containing the timestamp
			TimeFormat: "2006-01-02 15:04:05 +000 UTC", // Provide the time format according to the timestamp column's format
		},
	}

	// Execute the query and retrieve the time series data
	metricValues, err := postgres.QueryTimeseries(ctx, param)
	require.NoError(t, err)

	for _, metric := range metricValues {
		t.Log(metric)
	}
}
