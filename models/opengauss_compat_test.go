package models_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// compatRecorder records every SQL statement and error GORM reports, so tests
// can assert sqlite/mysql paths never run the openGauss version() probe.
type compatRecorder struct {
	logger.Interface
	mu      sync.Mutex
	queries []string
	errs    []string
}

func newCompatRecorder() *compatRecorder {
	return &compatRecorder{Interface: logger.Default.LogMode(logger.Silent)}
}

func (r *compatRecorder) LogMode(logger.LogLevel) logger.Interface { return r }

func (r *compatRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.mu.Lock()
	r.queries = append(r.queries, sql)
	if err != nil {
		r.errs = append(r.errs, err.Error())
	}
	r.mu.Unlock()
}

func (r *compatRecorder) Error(_ context.Context, msg string, data ...interface{}) {
	r.mu.Lock()
	r.errs = append(r.errs, fmt.Sprintf(msg, data...))
	r.mu.Unlock()
}

func (r *compatRecorder) Warn(_ context.Context, _ string, _ ...interface{}) {}
func (r *compatRecorder) Info(_ context.Context, _ string, _ ...interface{}) {}

func (r *compatRecorder) hasQuery(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.queries {
		if strings.Contains(strings.ToLower(q), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func (r *compatRecorder) snapshot() (queries, errs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.queries...), append([]string(nil), r.errs...)
}

func newCompatSQLiteCtx(t *testing.T, rec *compatRecorder) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: rec})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.EmbeddedProduct{}, &models.DingtalkGroup{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return ctx.NewContext(context.Background(), db, true)
}

// Regression: the four upsert/bind paths must gate the openGauss probe behind
// the postgres dialector. On sqlite they must not run version() (unsupported)
// and must not emit an ERROR log, while still writing data correctly.
func TestOpenGaussProbeGatedOnSQLite(t *testing.T) {
	rec := newCompatRecorder()
	c := newCompatSQLiteCtx(t, rec)

	if err := models.AddEmbeddedProduct(c, []models.EmbeddedProduct{
		{ID: 1, Name: "probe", URL: "http://example.com"},
	}); err != nil {
		t.Fatalf("AddEmbeddedProduct on sqlite: %v", err)
	}
	if err := models.UpsertDingtalkGroupInstall(c, &models.DingtalkGroup{
		ClientID: "client-1", OpenConversationID: "conv-1", Title: "group",
	}); err != nil {
		t.Fatalf("UpsertDingtalkGroupInstall on sqlite: %v", err)
	}

	queries, errs := rec.snapshot()
	if rec.hasQuery("select version()") {
		t.Fatalf("version() probe ran on sqlite; queries: %v", queries)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected SQL errors on sqlite: %v", errs)
	}

	var count int64
	if err := c.DB.Model(&models.EmbeddedProduct{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected 1 embedded_product row, got %d (err=%v)", count, err)
	}
	var dcount int64
	if err := c.DB.Model(&models.DingtalkGroup{}).Count(&dcount).Error; err != nil || dcount != 1 {
		t.Fatalf("expected 1 dingtalk_group row, got %d (err=%v)", dcount, err)
	}
}
