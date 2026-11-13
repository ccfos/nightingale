package doris

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/stretchr/testify/require"
)

// metadataConnector scripts database/sql results without a server or a global driver registration.
type metadataConnector struct {
	columns []string
	rows    [][]driver.Value
	queries []string
	err     error
}

func (c *metadataConnector) Connect(context.Context) (driver.Conn, error) {
	return &metadataConn{owner: c}, nil
}
func (c *metadataConnector) Driver() driver.Driver { return metadataDriver{} }

type metadataDriver struct{}

func (metadataDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type metadataConn struct{ owner *metadataConnector }

func (*metadataConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*metadataConn) Close() error              { return nil }
func (*metadataConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c *metadataConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.owner.queries = append(c.owner.queries, query)
	if c.owner.err != nil {
		return nil, c.owner.err
	}
	return &metadataRows{columns: c.owner.columns, rows: c.owner.rows}, nil
}

type metadataRows struct {
	columns []string
	rows    [][]driver.Value
	next    int
}

func (r *metadataRows) Columns() []string { return r.columns }
func (*metadataRows) Close() error        { return nil }
func (r *metadataRows) Next(dest []driver.Value) error {
	if r.next == len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.next])
	r.next++
	return nil
}

func metadataDoris(t *testing.T, database string, script *metadataConnector) *Doris {
	t.Helper()
	db := sql.OpenDB(script)
	d := &Doris{Addr: "metadata-" + t.Name(), User: "reader", Timeout: 5000}
	key := d.Addr + ":reader:"
	if database != "" {
		key += ":" + database
	}
	previous, existed := pool.PoolClient.Load(key)
	pool.PoolClient.Store(key, db)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
		require.NoError(t, db.Close())
	})
	return d
}

func TestShowDatabases_DecodesDatabaseNames(t *testing.T) {
	script := &metadataConnector{
		columns: []string{"Database"}, rows: [][]driver.Value{{"metrics"}, {[]byte("logs")}},
	}
	d := metadataDoris(t, "", script)

	names, err := d.ShowDatabases(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"metrics", "logs"}, names)
	require.Equal(t, []string{"SHOW DATABASES"}, script.queries)
}

func TestShowTables_QuotesDatabaseAndDecodesNames(t *testing.T) {
	script := &metadataConnector{columns: []string{"Tables_in_metrics"}, rows: [][]driver.Value{{"cpu"}, {"memory"}}}
	d := metadataDoris(t, "metrics", script)

	names, err := d.ShowTables(context.Background(), "metrics")

	require.NoError(t, err)
	require.Equal(t, []string{"cpu", "memory"}, names)
	require.Equal(t, []string{"SHOW TABLES IN `metrics`"}, script.queries)
}

func TestShowResources_DeduplicatesNamesAcrossRows(t *testing.T) {
	script := &metadataConnector{
		columns: []string{"Name", "ResourceType"},
		rows:    [][]driver.Value{{"archive", "S3"}, {[]byte("archive"), "S3"}, {[]byte("backups"), "S3"}},
	}
	d := metadataDoris(t, "", script)

	names, err := d.ShowResources(context.Background(), "S3")

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"archive", "backups"}, names)
	require.Equal(t, []string{"SHOW RESOURCES WHERE RESOURCETYPE = 'S3'"}, script.queries)
}

func TestShowResources_RejectsUnknownResourceTypeBeforeQuerying(t *testing.T) {
	script := &metadataConnector{}
	d := metadataDoris(t, "", script)

	names, err := d.ShowResources(context.Background(), "S3'; DROP TABLE users; --")

	require.ErrorContains(t, err, "invalid resource type")
	require.Nil(t, names)
	require.Empty(t, script.queries)
}

func TestDescTable_MapsColumnTypesAndIndexability(t *testing.T) {
	tests := []struct {
		typ       string
		want      string
		indexable bool
	}{
		{typ: "DOUBLE", want: types.LogExtractValueTypeFloat, indexable: true},
		{typ: "datetime", want: types.LogExtractValueTypeDate},
		{typ: "date", want: types.LogExtractValueTypeDate},
		{typ: "text", want: types.LogExtractValueTypeText, indexable: true},
		{typ: "BIGINT", want: types.LogExtractValueTypeLong, indexable: true},
		{typ: "datetimev2(3)", want: types.LogExtractValueTypeDate},
		{typ: "varchar(64)", want: types.LogExtractValueTypeText, indexable: true},
		{typ: "char(8)", want: types.LogExtractValueTypeText, indexable: true},
		{typ: "decimal(10,2)", want: types.LogExtractValueTypeFloat, indexable: true},
		{typ: "JSON", want: "JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			script := &metadataConnector{
				columns: []string{"Field", "Type", "Null", "Key", "Default", "Extra"},
				rows:    [][]driver.Value{{"value", tt.typ, "YES", "", nil, ""}},
			}
			d := metadataDoris(t, "metrics", script)

			columns, err := d.DescTable(context.Background(), "metrics", "samples")

			require.NoError(t, err)
			require.Equal(t, []*types.ColumnProperty{{
				Field: "value", Type: tt.typ, Type2: tt.want, Indexable: tt.indexable,
			}}, columns)
			require.Equal(t, []string{"DESCRIBE `metrics`.`samples`"}, script.queries)
		})
	}
}

func TestShowIndexes_UsesColumnNamesRatherThanColumnOrder(t *testing.T) {
	script := &metadataConnector{
		columns: []string{"Index_Type", "ignored", "Column_Name", "Key_Name"},
		rows: [][]driver.Value{
			{"INVERTED", int64(0), []byte("message"), "idx_message"},
			{"BITMAP", int64(0), "status", []byte("idx_status")},
			{"INVERTED", int64(0), "", "expression_index"},
		},
	}
	d := metadataDoris(t, "logs", script)

	indexes, err := d.ShowIndexes(context.Background(), "logs", "events")

	require.NoError(t, err)
	require.Equal(t, []TableIndexInfo{
		{ColumnName: "message", IndexName: "idx_message", IndexType: "INVERTED"},
		{ColumnName: "status", IndexName: "idx_status", IndexType: "BITMAP"},
	}, indexes)
	require.Len(t, script.queries, 1)
	require.Equal(t, "SHOW INDEX FROM `logs`.`events`", strings.Join(strings.Fields(script.queries[0]), " "))
}

func TestMetadataQueries_PropagateDatabaseQueryErrors(t *testing.T) {
	want := errors.New("metadata query failed")
	script := &metadataConnector{err: want}
	d := metadataDoris(t, "metrics", script)
	// ShowDatabases and ShowResources connect without selecting a database.
	key := d.Addr + ":reader:"
	db, ok := pool.PoolClient.Load(key + ":metrics")
	require.True(t, ok)
	previous, existed := pool.PoolClient.Load(key)
	pool.PoolClient.Store(key, db)
	t.Cleanup(func() {
		pool.PoolClient.Delete(key)
		if existed {
			pool.PoolClient.Store(key, previous)
		}
	})
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "databases", run: func() error { _, err := d.ShowDatabases(context.Background()); return err }},
		{name: "resources", run: func() error { _, err := d.ShowResources(context.Background(), "S3"); return err }},
		{name: "tables", run: func() error { _, err := d.ShowTables(context.Background(), "metrics"); return err }},
		{
			name: "columns",
			run:  func() error { _, err := d.DescTable(context.Background(), "metrics", "samples"); return err },
		},
		{
			name: "indexes",
			run:  func() error { _, err := d.ShowIndexes(context.Background(), "metrics", "samples"); return err },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			require.ErrorIs(t, err, want)
		})
	}
}
