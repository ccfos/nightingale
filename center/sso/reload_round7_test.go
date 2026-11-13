package sso

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReload_ReappliesTemplatesWhenVariableCacheChanges(t *testing.T) {
	client, c := reloadTestClient(t, []models.SsoConfig{{
		Name: "OAuth2", Content: "Enable = true\nDisplayName = '{{.login_name}}'", UpdateAt: 1,
	}})
	require.NoError(t, client.reload(c))
	require.Equal(t, "Company login", client.OAuth2.GetDisplayName())
	client.configCache.Set(map[string]string{"login_name": "Updated login"}, 1, 3)

	err := client.reload(c)

	require.NoError(t, err)
	require.Equal(t, "Updated login", client.OAuth2.GetDisplayName())
	require.Equal(t, int64(1), client.LastUpdateTime)
	require.Equal(t, int64(3), client.configLastUpdateTime)
}

func TestReload_PreservesProviderWhenConfigListCannotBeRead(t *testing.T) {
	client, c := reloadTestClient(t, []models.SsoConfig{{Name: "OAuth2", Content: OAuth2, UpdateAt: 1}})
	original := client.OAuth2
	failure := errors.New("config list unavailable")
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register("round7:fail_config_list", func(db *gorm.DB) {
		db.AddError(failure)
	}))
	t.Cleanup(func() { c.DB.Callback().Query().Remove("round7:fail_config_list") })

	err := client.reload(c)

	require.ErrorIs(t, err, failure)
	require.Same(t, original, client.OAuth2)
	require.Zero(t, client.LastUpdateTime)
	require.Zero(t, client.configLastUpdateTime)
}
