package sso

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/dingtalk"
	"github.com/ccfos/nightingale/v6/pkg/feishu"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func reloadTestClient(t *testing.T, configs []models.SsoConfig) (*SsoClient, *ctx.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.SsoConfig{}))
	require.NoError(t, db.Create(&configs).Error)
	cache := &memsto.ConfigCache{}
	cache.Set(map[string]string{"login_name": "Company login"}, 1, 2)
	oidcClient, err := oidcx.New(oidcx.Config{})
	require.NoError(t, err)
	ldapClient := ldapx.New(ldapx.Config{})
	t.Cleanup(ldapClient.Ticker.Stop)
	client := &SsoClient{
		LDAP: ldapClient, OIDC: oidcClient, CAS: cas.New(cas.Config{}),
		OAuth2: oauth2x.New(oauth2x.Config{}), configCache: cache,
	}
	return client, &ctx.Context{Ctx: context.Background(), DB: db, IsCenter: true}
}

func TestReload_AppliesProviderConfigurations(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`,
			server.URL, server.URL+"/authorize", server.URL+"/token", server.URL+"/keys")
	})
	client, ctx := reloadTestClient(t, []models.SsoConfig{
		{Name: "LDAP", Content: "Enable = true\nHost = 'localhost'\nDefaultTeams = [3, 7]", UpdateAt: 1},
		{
			Name: "OIDC", UpdateAt: 1,
			Content: fmt.Sprintf("Enable = true\nSsoAddr = %q\nDisplayName = 'OIDC login'", server.URL),
		},
		{Name: "CAS", Content: "Enable = true\nDisplayName = 'CAS login'", UpdateAt: 1},
		{Name: "OAuth2", Content: "Enable = true\nDisplayName = 'OAuth login'\nDefaultTeams = [5]", UpdateAt: 1},
		{Name: "dingtalk", Content: `{"enable":true,"display_name":"DingTalk login"}`, UpdateAt: 1},
		{Name: "feishu", Content: `{"enable":true,"display_name":"Feishu login"}`, UpdateAt: 1},
	})

	err := client.reload(ctx)

	require.NoError(t, err)
	require.True(t, client.LDAP.Enable)
	require.Equal(t, "localhost", client.LDAP.Host)
	require.Equal(t, []int64{3, 7}, client.LDAP.DefaultTeams)
	require.Equal(t, "OIDC login", client.OIDC.GetDisplayName())
	require.Equal(t, "CAS login", client.CAS.GetDisplayName())
	require.Equal(t, "OAuth login", client.OAuth2.GetDisplayName())
	require.Equal(t, []int64{5}, client.OAuth2.GetDefaultTeams())
	require.Equal(t, "DingTalk login", client.DingTalk.GetDisplayName())
	require.Equal(t, "Feishu login", client.FeiShu.GetDisplayName())
	require.Equal(t, int64(1), client.LastUpdateTime)
	require.Equal(t, int64(2), client.configLastUpdateTime)
}

func TestReload_UpdatesExistingJSONProviders(t *testing.T) {
	client, ctx := reloadTestClient(t, []models.SsoConfig{
		{Name: "dingtalk", Content: `{"enable":true,"display_name":"Updated DingTalk"}`, UpdateAt: 1},
		{Name: "feishu", Content: `{"enable":true,"display_name":"Updated Feishu"}`, UpdateAt: 1},
	})
	client.DingTalk = dingtalk.New(dingtalk.Config{Enable: true, DisplayName: "Old"})
	client.FeiShu = feishu.New(feishu.Config{Enable: true, DisplayName: "Old"})
	dingTalk, feiShu := client.DingTalk, client.FeiShu

	err := client.reload(ctx)

	require.NoError(t, err)
	require.Same(t, dingTalk, client.DingTalk)
	require.Same(t, feiShu, client.FeiShu)
	require.Equal(t, "Updated DingTalk", client.DingTalk.GetDisplayName())
	require.Equal(t, "Updated Feishu", client.FeiShu.GetDisplayName())
}

func TestReload_RemovesAbsentJSONProviders(t *testing.T) {
	client, ctx := reloadTestClient(t, []models.SsoConfig{{Name: "LDAP", Content: "Enable = false", UpdateAt: 1}})
	client.DingTalk = dingtalk.New(dingtalk.Config{Enable: true})
	client.FeiShu = feishu.New(feishu.Config{Enable: true})

	err := client.reload(ctx)

	require.NoError(t, err)
	require.Nil(t, client.DingTalk)
	require.Nil(t, client.FeiShu)
}

func TestReload_PreservesProvidersOnInvalidConfig(t *testing.T) {
	for _, name := range []string{"LDAP", "OIDC", "CAS", "OAuth2", "dingtalk", "feishu"} {
		t.Run(name, func(t *testing.T) {
			client, ctx := reloadTestClient(t, []models.SsoConfig{{Name: name, Content: "invalid = [", UpdateAt: 1}})
			client.DingTalk = dingtalk.New(dingtalk.Config{Enable: true, DisplayName: "DingTalk login"})
			client.FeiShu = feishu.New(feishu.Config{Enable: true, DisplayName: "Feishu login"})

			err := client.reload(ctx)

			require.NoError(t, err)
			switch name {
			case "LDAP":
				require.False(t, client.LDAP.Enable)
			case "OIDC":
				require.False(t, client.OIDC.Enable)
			case "CAS":
				require.False(t, client.CAS.Enable)
			case "OAuth2":
				require.False(t, client.OAuth2.Enable)
			case "dingtalk":
				require.Equal(t, "DingTalk login", client.DingTalk.GetDisplayName())
			case "feishu":
				require.Equal(t, "Feishu login", client.FeiShu.GetDisplayName())
			}
		})
	}
}

func TestReload_SkipsUnchangedConfiguration(t *testing.T) {
	client, ctx := reloadTestClient(t, []models.SsoConfig{{Name: "feishu", Content: "invalid", UpdateAt: 1}})
	client.LastUpdateTime, client.configLastUpdateTime = 1, 2
	client.FeiShu = feishu.New(feishu.Config{Enable: true, DisplayName: "Preserved"})

	err := client.reload(ctx)

	require.NoError(t, err)
	require.Equal(t, "Preserved", client.FeiShu.GetDisplayName())
}

func TestReload_ReportsDatabaseFailure(t *testing.T) {
	client, ctx := reloadTestClient(t, []models.SsoConfig{{Name: "LDAP", Content: LDAP, UpdateAt: 1}})
	require.NoError(t, ctx.DB.Migrator().DropTable(&models.SsoConfig{}))

	err := client.reload(ctx)

	require.ErrorContains(t, err, "sso_config")
	require.Zero(t, client.LastUpdateTime)
}
