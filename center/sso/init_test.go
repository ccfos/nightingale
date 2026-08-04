package sso

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
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

func saveSsoConfig(t *testing.T, c *ctx.Context, name, content string) {
	t.Helper()

	cfg := models.SsoConfig{Name: name, Content: content, UpdateAt: time.Now().Unix()}
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

func saveOIDCConfig(t *testing.T, c *ctx.Context, content string) {
	t.Helper()
	saveSsoConfig(t, c, "OIDC", content)
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
	// 读不到库就不知道该信任谁，先收紧：管理员可能刚关掉 OIDC 或换掉不再受信的 IdP
	if s.OIDC.Ready() {
		t.Error("oidc must not keep accepting logins with a config we can no longer confirm")
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

const ldapTOMLWithSyncInterval = `
Enable = true
Host = 'ldap.example.org'
Port = 389
SyncAddUsers = false
SyncDelUsers = false
SyncInterval = 1
AuthFilter = '(&(uid=%s))'
`

// OIDC 长期对接不上时，每个周期的重试不能顺带把 LDAP 也重载一遍：LDAP 重载会重置用户
// 同步 Ticker，间隔稍长的 LDAP 同步就永远等不到触发
func TestOIDCRetryDoesNotStarveLdapTicker(t *testing.T) {
	s, c := newTestSsoClient(t)
	s.LDAP = ldapx.New(ldapx.Config{})

	saveSsoConfig(t, c, "LDAP", ldapTOMLWithSyncInterval)
	saveOIDCConfig(t, c, oidcTOML(true, newTestIdP(t)+"/not-an-idp"))

	// 第一轮：配置有变更，整个 reload 跑一遍，LDAP 同步 Ticker 被设为 1 秒
	if err := s.reload(c); err != nil {
		t.Fatalf("first reload: %v", err)
	}
	if !s.oidcNeedsRetry() {
		t.Fatal("oidc should be marked for retry after failing to reach the idp")
	}

	// 之后的周期里配置没再变，只有 OIDC 在反复重试
	s.LDAP.Host = "sentinel.example.org"
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.reload(c) // IdP 一直不可达，这里必然失败，只关心它有没有波及 LDAP
		time.Sleep(200 * time.Millisecond)
	}

	if !s.oidcNeedsRetry() {
		t.Error("oidc should still be marked for retry while the idp stays unreachable")
	}
	if s.LDAP.Host != "sentinel.example.org" {
		t.Error("ldap must not be reloaded when only oidc needs a retry")
	}
	select {
	case <-s.LDAP.Ticker.C:
	default:
		t.Error("ldap sync ticker should still fire while oidc keeps retrying")
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
