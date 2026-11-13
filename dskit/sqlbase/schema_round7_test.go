package sqlbase

import (
	"context"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Use SQLite to provide metadata rows while selecting the production schema projection by dialect name.
type metadataDialect struct {
	gorm.Dialector
	name string
}

func (d metadataDialect) Name() string { return d.name }

func TestDescTable_ProjectsDialectMetadataIntoColumnProperties(t *testing.T) {
	cases := []struct {
		name, query, dbType, valueType string
		indexable                      bool
	}{
		{
			name: "mysql", query: "SELECT 'count', 'BIGINT', 'YES', NULL, NULL, NULL",
			dbType: "BIGINT", valueType: types.LogExtractValueTypeLong, indexable: true,
		},
		{
			name: "postgres", query: "SELECT 'count', 'varchar(32)', 'YES', NULL",
			dbType: "varchar(32)", valueType: types.LogExtractValueTypeText, indexable: true,
		},
		{
			name: "sqlserver", query: "SELECT 'count', 'FLOAT', 'YES', NULL",
			dbType: "FLOAT", valueType: types.LogExtractValueTypeFloat, indexable: true,
		},
		{
			name: "oracle", query: "SELECT 'count', 'DATE', 'YES'",
			dbType: "DATE", valueType: types.LogExtractValueTypeDate,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := queryTestDB(t)
			db.Dialector = metadataDialect{Dialector: db.Dialector, name: tc.name}

			columns, err := DescTable(context.Background(), db, tc.query)

			require.NoError(t, err)
			require.Equal(t, []*types.ColumnProperty{{
				Field: "count", Type: tc.dbType, Type2: tc.valueType, Indexable: tc.indexable,
			}}, columns)
		})
	}
}

func TestDescTable_ReportsQueryFailure(t *testing.T) {
	db := queryTestDB(t)

	columns, err := DescTable(context.Background(), db, "SELECT * FROM missing_table")

	require.ErrorContains(t, err, "missing_table")
	require.Nil(t, columns)
}

func TestParseTime_AcceptsAdditionalSQLAndNumericStrings(t *testing.T) {
	cases := []struct {
		name, value string
		want        time.Time
	}{
		{name: "SQL datetime", value: "2024-05-14 12:34:56", want: time.Date(2024, 5, 14, 12, 34, 56, 0, time.UTC)},
		{
			name: "SQL microseconds", value: "2024-05-14 12:34:56.123456",
			want: time.Date(2024, 5, 14, 12, 34, 56, 123456000, time.UTC),
		},
		{name: "integer timestamp", value: "1715642135", want: time.Unix(1715642135, 0)},
		{name: "scientific timestamp", value: "1.715642135e9", want: time.Unix(1715642135, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTime(tc.value, "")

			require.NoError(t, err)
			require.True(t, tc.want.Equal(got), "got %s, want %s", got, tc.want)
		})
	}
}

func TestParseTime_RejectsUnrecognizedString(t *testing.T) {
	got, err := ParseTime("not-a-time", "")

	require.ErrorContains(t, err, "failed to parse time 'not-a-time'")
	require.True(t, got.IsZero())
}

func TestShowTables_ReportsUnexpectedColumnCount(t *testing.T) {
	db := queryTestDB(t)

	names, err := ShowTables(context.Background(), db, "SELECT 'table', 'extra'")

	require.ErrorContains(t, err, "destination arguments")
	require.Nil(t, names)
}

func TestShowDatabases_ReportsUnexpectedColumnCount(t *testing.T) {
	db := queryTestDB(t)

	names, err := ShowDatabases(context.Background(), db, "SELECT 'database', 'extra'")

	require.ErrorContains(t, err, "destination arguments")
	require.Nil(t, names)
}
