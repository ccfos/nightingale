package ormx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// probeRecorder captures the SQL GORM emits so tests can assert the version()
// probe is skipped on non-postgres dialectors.
type probeRecorder struct {
	logger.Interface
	mu      sync.Mutex
	queries []string
}

func newProbeRecorder() *probeRecorder {
	return &probeRecorder{Interface: logger.Default.LogMode(logger.Silent)}
}

func (r *probeRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

func (r *probeRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.queries = append(r.queries, sql)
	r.mu.Unlock()
}

func (r *probeRecorder) ran(query string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.queries {
		if strings.Contains(strings.ToLower(q), strings.ToLower(query)) {
			return true
		}
	}
	return false
}

func (r *probeRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.queries...)
}

func TestIsOpenGaussVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    bool
	}{
		{"openGauss v5", "openGauss 5.0.0 build compiled at 2023-12-28", true},
		{"PanWeiDB 2.0 banner", "PanWeiDB(openGauss) 2.0.0 build 9a7e96bc", true},
		{"case insensitive", "OPENGAUSS 5.1.0", true},
		{"standard PostgreSQL", "PostgreSQL 15.2 on x86_64-pc-linux-gnu", false},
		{"MySQL", "8.0.32", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOpenGaussVersion(tc.version); got != tc.want {
				t.Fatalf("isOpenGaussVersion(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

// Regression: mysql/sqlite must never execute SELECT version() (sqlite has no
// such function), otherwise every business write emits an ERROR log.
func TestNeedsOpenGaussCompatSkipsProbeOnSQLite(t *testing.T) {
	rec := newProbeRecorder()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: rec})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if got := db.Dialector.Name(); got != "sqlite" {
		t.Fatalf("dialector = %q, want sqlite", got)
	}
	if NeedsOpenGaussCompat(db) {
		t.Fatal("NeedsOpenGaussCompat returned true on sqlite")
	}
	if rec.ran("select version()") {
		t.Fatalf("version() probe ran on sqlite; queries: %v", rec.all())
	}
}

// Sentinel for the regression test above: prove the recorder actually sees
// the version() probe, so "no probe recorded" is meaningful and not vacuous.
func TestProbeRecorderDetectsVersionQuery(t *testing.T) {
	rec := newProbeRecorder()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: rec})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	IsOpenGaussDB(db)
	if !rec.ran("select version()") {
		t.Fatalf("recorder failed to capture the version() probe; queries: %v", rec.all())
	}
}

// A failing probe must fail closed instead of panicking or propagating.
func TestIsOpenGaussDBReturnsFalseOnProbeError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if IsOpenGaussDB(db) {
		t.Fatal("IsOpenGaussDB returned true when version() is unavailable")
	}
}
