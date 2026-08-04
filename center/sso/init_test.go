package sso

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestSsoClient(t *testing.T) (*SsoClient, *ctx.Context) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SsoConfig{}); err != nil {
		t.Fatalf("migrate sso_config: %v", err)
	}

	s := &SsoClient{OIDC: &oidcx.SsoClient{}, configCache: &memsto.ConfigCache{}}
	return s, ctx.NewContext(context.Background(), db, true)
}

// newTestIdP 起一个只提供 discovery 文档的 IdP，够 oidc.NewProvider 用
func newTestIdP(t *testing.T) string {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`,
			srv.URL, srv.URL+"/auth", srv.URL+"/token", srv.URL+"/jwks")
	})
	return srv.URL
}

func saveOIDCConfig(t *testing.T, c *ctx.Context, content string) {
	t.Helper()

	cfg := models.SsoConfig{Name: "OIDC", Content: content}
	existing, err := cfg.Query(c)
	if err != nil {
		if err := cfg.Create(c); err != nil {
			t.Fatalf("create sso config: %v", err)
		}
		return
	}

	cfg.Id = existing.Id
	if err := cfg.Update(c); err != nil {
		t.Fatalf("update sso config: %v", err)
	}
}

func oidcTOML(enable bool, ssoAddr string) string {
	return fmt.Sprintf(`Enable = %t
DisplayName = 'Sign in with OIDC'
RedirectURL = 'http://n9e.com/callback'
SsoAddr = '%s'
ClientId = 'n9e'
`, enable, ssoAddr)
}

// 定期 reload 读到旧配置后可能停顿很久（网络、调度），期间管理员保存并发布了新配置；
// 定期 reload 恢复执行时必须以库里的最新配置为准，不能把它手里那份旧配置发布回去
func TestReloadOIDCPublishesLatestConfigNotCallerSnapshot(t *testing.T) {
	s, c := newTestSsoClient(t)
	issuer := newTestIdP(t)

	// 定期 reload 的视角：此刻库里是一份启用的 OIDC 配置
	saveOIDCConfig(t, c, oidcTOML(true, issuer))
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload enabled oidc: %v", err)
	}
	if !s.OIDC.Ready() {
		t.Fatal("oidc should be ready after loading a reachable idp")
	}

	// 管理员把 OIDC 关掉并立即生效
	saveOIDCConfig(t, c, oidcTOML(false, issuer))
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload disabled oidc: %v", err)
	}
	if s.OIDC.Ready() {
		t.Fatal("oidc should be disabled after the admin turned it off")
	}

	// 定期 reload 这才轮到 OIDC：它不携带任何配置快照，只会重新读库，禁用状态得以保持
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload from the periodic worker: %v", err)
	}
	if s.OIDC.Ready() {
		t.Error("a later reload must not resurrect the config the admin just replaced")
	}
	if s.oidcNeedsRetry() {
		t.Error("a disabled oidc does not need retrying")
	}
}

// IdP 暂时不可达时要记下重试标记，否则 update_at 只有秒级精度，配置没再变过就没人重试了
func TestReloadOIDCMarksRetryWhenIdPUnreachable(t *testing.T) {
	s, c := newTestSsoClient(t)

	dead := newTestIdP(t)
	saveOIDCConfig(t, c, oidcTOML(true, dead+"/not-an-idp"))
	if err := s.ReloadOIDC(c); err == nil {
		t.Fatal("expected an error when the idp is unreachable")
	}
	if !s.oidcNeedsRetry() {
		t.Error("a failed oidc reload must be marked for retry")
	}
	if s.OIDC.Ready() {
		t.Error("oidc must not be ready when discovery failed")
	}

	// IdP 恢复（这里换成一个可用的 issuer），下一轮重试即自动补齐，无需重启进程
	saveOIDCConfig(t, c, oidcTOML(true, newTestIdP(t)))
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload after the idp came back: %v", err)
	}
	if !s.OIDC.Ready() {
		t.Error("oidc should recover on the next retry")
	}
	if s.oidcNeedsRetry() {
		t.Error("retry flag should be cleared once the reload succeeded")
	}
}

// 定期 reload 已经成功读过一次配置，ReloadOIDC 内部的第二次读库若失败，必须置位重试
// 标记：调用方随后照样推进更新时间水位，不置位的话下个周期会直接跳过，这次配置变更
// （这里是「关闭 OIDC」）就永远不会生效
func TestReloadOIDCRetriesWhenConfigQueryFails(t *testing.T) {
	s, c := newTestSsoClient(t)
	issuer := newTestIdP(t)

	saveOIDCConfig(t, c, oidcTOML(true, issuer))
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload enabled oidc: %v", err)
	}
	if !s.OIDC.Ready() {
		t.Fatal("oidc should be ready after loading a reachable idp")
	}

	if err := models.DB(c).Exec("DROP TABLE sso_config").Error; err != nil {
		t.Fatalf("drop sso_config: %v", err)
	}
	if err := s.ReloadOIDC(c); err == nil {
		t.Fatal("expected an error when reading the config fails")
	}
	if !s.oidcNeedsRetry() {
		t.Error("a failed config read must keep the periodic reload retrying")
	}

	// 库恢复后（且此时的配置是「关闭 OIDC」），下一轮重试必须把它应用上
	if err := models.DB(c).AutoMigrate(&models.SsoConfig{}); err != nil {
		t.Fatalf("recreate sso_config: %v", err)
	}
	saveOIDCConfig(t, c, oidcTOML(false, issuer))
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload after the database recovered: %v", err)
	}
	if s.OIDC.Ready() {
		t.Error("oidc should be disabled once the pending config change is finally applied")
	}
	if s.oidcNeedsRetry() {
		t.Error("retry flag should be cleared once the reload succeeded")
	}
}

// 库里没有 OIDC 记录时不该报错，也不该一直空转重试
func TestReloadOIDCWithoutConfig(t *testing.T) {
	s, c := newTestSsoClient(t)

	s.oidcNotReady = true
	if err := s.ReloadOIDC(c); err != nil {
		t.Fatalf("reload without an oidc config: %v", err)
	}
	if s.oidcNeedsRetry() {
		t.Error("retry flag should be cleared when there is no oidc config at all")
	}
}
