// @Author: Ciusyan 5/11/24

package mysql

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	param := &sqlbase.QueryParam{
		Sql: "SELECT * FROM students WHERE id > 10900",
		Keys: types.Keys{
			ValueKey:   "",
			LabelKey:   "",
			TimeKey:    "",
			TimeFormat: "",
		},
	}

	rows, err := mysql.Query(ctx, param)
	require.NoError(t, err)
	for _, row := range rows {
		t.Log(row)
	}
}

func TestQueryTimeseries(t *testing.T) {
	ctx := context.Background()
	settings := `{"mysql.addr":"localhost:3306","mysql.user":"root","mysql.password":"root","mysql.maxIdleConns":5,"mysql.maxOpenConns":10,"mysql.connMaxLifetime":30}`
	mysql, err := NewMySQLWithSettings(ctx, settings)
	require.NoError(t, err)

	// Prepare a test query parameter
	param := &sqlbase.QueryParam{
		Sql: "SELECT id, grade, student_name, a_grade, update_time FROM students WHERE grade > 20000", // Modify SQL query to select specific columns
		Keys: types.Keys{
			ValueKey:   "grade a_grade",                 // Set the value key to the column name containing the metric value
			LabelKey:   "id student_name",               // Set the label key to the column name containing the metric label
			TimeKey:    "update_time",                   // Set the time key to the column name containing the timestamp
			TimeFormat: "2006-01-02 15:04:05 +0000 UTC", // Provide the time format according to the timestamp column's format
		},
	}

	// Execute the query and retrieve the time series data
	metricValues, err := mysql.QueryTimeseries(ctx, param)
	require.NoError(t, err)

	for _, metric := range metricValues {
		t.Log(metric)
	}
}
