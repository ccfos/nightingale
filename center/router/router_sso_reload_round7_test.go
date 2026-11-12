package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/stretchr/testify/require"
)

func TestSsoConfigUpdate_ReloadsLocalProviderConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		content string
		check   func(*testing.T, *Router)
	}{
		{
			name: "LDAP", content: "Enable = false\nHost = '127.0.0.1'",
			check: func(t *testing.T, rt *Router) { require.False(t, rt.Sso.LDAP.Enable) },
		},
		{
			name: "OIDC", content: "Enable = false\nDisplayName = 'Disabled OIDC'",
			check: func(t *testing.T, rt *Router) {
				require.NotNil(t, rt.Sso.OIDC)
				require.False(t, rt.Sso.OIDC.Enable)
			},
		},
		{
			name: "CAS", content: "Enable = true\nSsoAddr = 'http://127.0.0.1'\nDisplayName = 'Updated CAS'",
			check: func(t *testing.T, rt *Router) { require.Equal(t, "Updated CAS", rt.Sso.CAS.GetDisplayName()) },
		},
		{
			name: "OAuth2", content: "Enable = true\nSsoAddr = 'http://127.0.0.1/authorize'\nDisplayName = 'Updated OAuth'",
			check: func(t *testing.T, rt *Router) { require.Equal(t, "Updated OAuth", rt.Sso.OAuth2.GetDisplayName()) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.SsoConfig{})
			rt.Sso = &sso.SsoClient{
				LDAP: ldapx.New(ldapx.Config{}), OIDC: &oidcx.SsoClient{}, CAS: cas.New(cas.Config{}),
				OAuth2: oauth2x.New(oauth2x.Config{}),
			}
			original := models.SsoConfig{Id: 1, Name: tc.name, Content: "Enable = false"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)

			form := models.SsoConfig{Id: 1, Name: tc.name, Content: tc.content}

			w := identityRequest(t, rt, rt.ssoConfigUpdate, form, "0", nil)

			require.Empty(t, identityReply(t, w, nil))
			var saved models.SsoConfig
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, tc.name, saved.Name)
			require.Equal(t, tc.content, saved.Content)
			tc.check(t, rt)
		})
	}
}
