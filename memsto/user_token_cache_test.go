package memsto

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestUserTokenCache(t *testing.T) (*UserTokenCacheType, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.User{Id: 1, Username: "alice", Roles: "Standard"}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// 不调用 NewUserTokenCache：它会启动后台同步协程，测试里手动调用 syncUserTokens
	utc := &UserTokenCacheType{
		ctx: ctx.NewContext(context.Background(), db, true),
		stats: &Stats{
			GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "d"}, []string{"name"}),
			GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "n"}, []string{"name"}),
		},
		tokens:         make(map[string]*models.User),
		tokensLastUsed: make(map[string]int64),
	}
	return utc, db
}

// #3457：两次同步之间新增一个 token、删除另一个，总数不变，
// 同步后新 token 必须生效、已删除的 token 必须失效。
func TestUserTokenCacheRotationWithUnchangedCount(t *testing.T) {
	utc, db := newTestUserTokenCache(t)
	c := utc.ctx

	oldToken, err := models.AddToken(c, "alice", "token-a", "a")
	if err != nil {
		t.Fatalf("add token a: %v", err)
	}
	if err := utc.syncUserTokens(); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if u := utc.GetByToken("token-a"); u == nil || u.Username != "alice" {
		t.Fatalf("token-a should authenticate after first sync, got %+v", u)
	}

	if _, err := models.AddToken(c, "alice", "token-b", "b"); err != nil {
		t.Fatalf("add token b: %v", err)
	}
	if _, err := models.DeleteToken(c, oldToken.Id, "alice"); err != nil {
		t.Fatalf("delete token a: %v", err)
	}
	var total int64
	db.Model(&models.UserToken{}).Count(&total)
	if total != 1 {
		t.Fatalf("token total = %d, want 1 (count unchanged)", total)
	}

	if err := utc.syncUserTokens(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if u := utc.GetByToken("token-b"); u == nil || u.Username != "alice" {
		t.Fatalf("new token-b should authenticate, got %+v", u)
	}
	if u := utc.GetByToken("token-a"); u != nil {
		t.Fatalf("deleted token-a should be rejected, got %+v", u)
	}
}

func TestUserTokenCacheSkipsReloadWhenUnchanged(t *testing.T) {
	utc, _ := newTestUserTokenCache(t)

	if _, err := models.AddToken(utc.ctx, "alice", "token-a", "a"); err != nil {
		t.Fatalf("add token: %v", err)
	}
	if err := utc.syncUserTokens(); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	sig := utc.statSig

	// 内容未变时应跳过重载：在缓存里塞一个标记，若被重载会被覆盖掉
	utc.Inject("marker", &models.User{Username: "marker"})
	if err := utc.syncUserTokens(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if utc.statSig != sig {
		t.Fatalf("signature changed without data change")
	}
	if utc.GetByToken("marker") == nil {
		t.Fatalf("cache was reloaded although tokens did not change")
	}
}
