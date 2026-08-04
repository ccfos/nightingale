package migrate

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlRecorder captures every statement gorm actually sends to the database.
type sqlRecorder struct {
	gormlogger.Interface
	mu   sync.Mutex
	sqls []string
}

func (r *sqlRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.mu.Lock()
	r.sqls = append(r.sqls, sql)
	r.mu.Unlock()
}

func (r *sqlRecorder) reset() {
	r.mu.Lock()
	r.sqls = nil
	r.mu.Unlock()
}

func (r *sqlRecorder) executed(fragment string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sql := range r.sqls {
		if strings.Contains(sql, fragment) {
			return true
		}
	}
	return false
}

// TestChainedSessionLeaksSQL pins the gorm semantics that let a background
// migration goroutine hang the whole process.
//
// A handle returned by a chain method (Set/Where/Table/...) has clone==0, so
// getInstance() hands back that very same *DB and every caller shares one
// Statement. While a statement is in flight its SQL sits on that Statement —
// processor.Execute only resets it once the statement returns — and any other
// user of the handle picks the SQL up through Statement.clone(), which copies
// SQL whenever it is non-empty, then executes it instead of its own query.
// That is how the main migration loop re-issued the async goroutine's
// CREATE INDEX on alert_his_event, blocked on the metadata lock forever, and
// left the HTTP port unbound.
//
// This test does NOT guard the asyncDB line in MigrateTables — it exercises
// gorm directly. What it protects is the assumption that line rests on: if a
// gorm bump ever changes whether Statement.clone() copies SQL, or whether
// Session with a Context clones the Statement at all, this fails loudly
// instead of silently resurrecting the hang.
//
// The in-flight statement is simulated by writing SQL onto a Statement, so the
// whole thing is deterministic and needs no concurrency.
func TestChainedSessionLeaksSQL(t *testing.T) {
	const sentinel = "SELECT 'leaked-sql-sentinel'"

	rec := &sqlRecorder{Interface: gormlogger.Discard}
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: rec})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("CREATE TABLE t (id integer)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	// The old shape: one clone==0 handle shared by the main flow and the
	// goroutine, so the goroutine's in-flight SQL sits on the Statement the
	// main flow is about to clone.
	shared := db.Set("gorm:table_options", "")
	shared.Statement.SQL.WriteString(sentinel)

	// This is the chain gorm's Migrator.ColumnTypes runs. It should query
	// table t; instead it inherits and executes the in-flight SQL.
	rec.reset()
	if rows, err := shared.Session(&gorm.Session{}).Table("t").Limit(1).Rows(); err == nil {
		rows.Close()
	}
	if !rec.executed("leaked-sql-sentinel") {
		t.Fatalf("expected the shared Statement's SQL to leak into the query, executed: %v", rec.sqls)
	}

	// The fix: the goroutine gets a Session with a non-nil Context, which
	// clones the Statement. Its in-flight SQL now lives on a Statement of its
	// own and the main flow cannot pick it up.
	parent := db.Set("gorm:table_options", "")
	async := parent.Session(&gorm.Session{Context: context.Background()})
	if async.Statement == parent.Statement {
		t.Fatal("Session with a Context must not share the Statement pointer")
	}
	async.Statement.SQL.WriteString(sentinel)

	rec.reset()
	if rows, err := parent.Session(&gorm.Session{}).Table("t").Limit(1).Rows(); err == nil {
		rows.Close()
	}
	if rec.executed("leaked-sql-sentinel") {
		t.Fatalf("main flow must not inherit the async handle's SQL, executed: %v", rec.sqls)
	}
	if !rec.executed("FROM `t`") {
		t.Fatalf("main flow should have queried t, executed: %v", rec.sqls)
	}
}

// TestMigrationDBIsolatesStatements guards migrationDB itself: statements run
// on the handle it returns must not leave their SQL or their Error behind for
// the next caller. The chained handle is asserted alongside it to show the
// failure mode being prevented -- on that one a single failed statement
// poisons everything that follows.
func TestMigrationDBIsolatesStatements(t *testing.T) {
	rec := &sqlRecorder{Interface: gormlogger.Discard}
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: rec})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// The failure mode: on a clone==0 handle the error sticks, and gorm's raw
	// callback then skips every later statement instead of running it.
	chained := db.Set("gorm:table_options", "")
	if err := chained.Exec("SELECT * FROM no_such_table").Error; err == nil {
		t.Fatal("expected the bogus statement to fail")
	}
	if chained.Error == nil {
		t.Fatal("precondition changed: a chained handle no longer keeps the error")
	}
	if err := chained.Exec("CREATE TABLE poisoned (id integer)").Error; err == nil {
		t.Fatal("precondition changed: a poisoned chained handle no longer blocks later statements")
	}
	if db.Migrator().HasTable("poisoned") {
		t.Fatal("precondition changed: the poisoned handle actually executed the statement")
	}

	// migrationDB's handle keeps errors local to the statement that caused them.
	h := migrationDB(db, "CHARSET=utf8mb4")
	if err := h.Exec("SELECT * FROM no_such_table").Error; err == nil {
		t.Fatal("expected the bogus statement to fail")
	}
	if h.Error != nil {
		t.Fatalf("migrationDB handle must not keep the error: %v", h.Error)
	}
	if h.Statement.SQL.Len() != 0 {
		t.Fatalf("migrationDB handle must not keep the SQL: %q", h.Statement.SQL.String())
	}
	if err := h.Exec("CREATE TABLE recovered (id integer)").Error; err != nil {
		t.Fatalf("later statements must still run: %v", err)
	}
	if !h.Migrator().HasTable("recovered") {
		t.Fatal("later statement did not reach the database")
	}
}
