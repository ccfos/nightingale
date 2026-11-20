package models

import (
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/dsauth"

	"github.com/stretchr/testify/require"
)

var cloudSecrets = []string{"basic-pass", "aws-secret", "oauth-secret", "azure-secret"}

func cloudAuthDatasource(cfg dsauth.Config) Datasource {
	return Datasource{
		Name:       "amp",
		PluginType: PROMETHEUS,
		HTTPJson:   HTTP{Url: "https://aps-workspaces.us-east-1.amazonaws.com/workspaces/ws-1"},
		AuthJson: Auth{
			BasicAuth: true, BasicAuthUser: "basic-user", BasicAuthPassword: "basic-pass",
			Config: cfg,
		},
	}
}

func TestDatasourceAuthLegacyJSONCompat(t *testing.T) {
	var a Auth
	require.NoError(t, json.Unmarshal([]byte(`{"basic_auth":true,"basic_auth_user":"u","basic_auth_password":"p"}`), &a))
	require.Equal(t, Auth{BasicAuth: true, BasicAuthUser: "u", BasicAuthPassword: "p"}, a)

	// legacy rows keep serializing without the new keys
	b, err := json.Marshal(a)
	require.NoError(t, err)
	require.JSONEq(t, `{"basic_auth":true,"basic_auth_user":"u","basic_auth_password":"p"}`, string(b))
}

func TestDatasourceAuthCloudJSONShape(t *testing.T) {
	raw := `{"basic_auth":false,"basic_auth_user":"","basic_auth_password":"","type":"sigv4",
		"sigv4":{"region":"us-east-1","service_name":"aps","credential_type":"default","role_arn":"arn:aws:iam::1:role/q","external_id":"x"}}`
	var a Auth
	require.NoError(t, json.Unmarshal([]byte(raw), &a))
	require.Equal(t, dsauth.TypeSigV4, a.Type)
	require.Equal(t, "arn:aws:iam::1:role/q", a.SigV4.RoleArn)
}

func TestDatasourceVerifyCloudAuthDropsBasic(t *testing.T) {
	ds := cloudAuthDatasource(dsauth.Config{Type: dsauth.TypeSigV4, SigV4: &dsauth.SigV4Config{Region: "us-east-1"},
		OAuth2: &dsauth.OAuth2Config{ClientSecret: "oauth-secret"}})
	require.NoError(t, ds.Verify())

	require.Empty(t, ds.AuthJson.BasicAuthUser)
	require.Empty(t, ds.AuthJson.BasicAuthPassword)
	require.Nil(t, ds.AuthJson.OAuth2, "sub-configs of other types must not be persisted")
	require.Equal(t, "aps", ds.AuthJson.SigV4.Service)
	require.NotContains(t, ds.Auth, "basic-pass")
	require.NotContains(t, ds.Auth, "oauth-secret")
}

func TestDatasourceVerifyBasicUnchanged(t *testing.T) {
	ds := cloudAuthDatasource(dsauth.Config{})
	ds.HTTPJson.Headers = map[string]string{"Authorization": "Bearer static"}
	require.NoError(t, ds.Verify())
	require.Equal(t, "basic-pass", ds.AuthJson.BasicAuthPassword)
}

func TestDatasourceVerifyRejectsAuthorizationHeaderWithCloudAuth(t *testing.T) {
	ds := cloudAuthDatasource(dsauth.Config{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{
		ClientId: "c", ClientSecret: "oauth-secret", TokenURL: "https://idp.example/token"}})
	ds.HTTPJson.Headers = map[string]string{"authorization ": "Bearer static"}
	require.ErrorContains(t, ds.Verify(), "Authorization header")
}

func TestDatasourceVerifyValidatesCloudAuth(t *testing.T) {
	ds := cloudAuthDatasource(dsauth.Config{Type: dsauth.TypeSigV4, SigV4: &dsauth.SigV4Config{}})
	require.ErrorContains(t, ds.Verify(), "region")
}

func TestDatasourceSecretsNotLeakedWithCloudAuth(t *testing.T) {
	configs := []dsauth.Config{
		{Type: dsauth.TypeSigV4, SigV4: &dsauth.SigV4Config{Region: "r", CredentialType: dsauth.AWSCredentialKeys, AccessKey: "AKID", SecretKey: "aws-secret"}},
		{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{ClientId: "c", ClientSecret: "oauth-secret", TokenURL: "https://idp/token"}},
		{Type: dsauth.TypeAzure, Azure: &dsauth.AzureConfig{TenantId: "t", ClientId: "c", ClientSecret: "azure-secret"}},
		{}, // basic only
	}
	for _, cfg := range configs {
		// ClearPlaintext runs after Encrypt, before the list is sent to edge.
		ds := cloudAuthDatasource(cfg)
		require.NoError(t, ds.FE2DB())
		ds.ClearPlaintext()
		payload, err := json.Marshal(ds)
		require.NoError(t, err)
		for _, secret := range cloudSecrets {
			require.NotContains(t, string(payload), secret, "ClearPlaintext type=%q", cfg.Type)
		}

		// RedactSecrets runs before returning datasources to non-admins.
		ds = cloudAuthDatasource(cfg)
		require.NoError(t, ds.FE2DB())
		ds.RedactSecrets()
		payload, err = json.Marshal(ds)
		require.NoError(t, err)
		for _, secret := range cloudSecrets {
			require.NotContains(t, string(payload), secret, "RedactSecrets type=%q", cfg.Type)
		}
	}
}

func TestDatasourceAuthBasicTypeStoredAsLegacy(t *testing.T) {
	ds := cloudAuthDatasource(dsauth.Config{Type: dsauth.TypeBasic})
	require.NoError(t, ds.Verify())
	require.Equal(t, Auth{BasicAuth: true, BasicAuthUser: "basic-user", BasicAuthPassword: "basic-pass"}, ds.AuthJson)
	require.NotContains(t, ds.Auth, `"type"`)
}
