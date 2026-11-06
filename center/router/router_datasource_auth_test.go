package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/dsauth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOAuth2Servers(t *testing.T, upstreamStatus int) (tokenURL, upstreamURL string) {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok","token_type":"bearer","expires_in":3600}`)
	}))
	t.Cleanup(tokenSrv.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(upstreamStatus)
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"scalar","result":[0,"2"]}}`)
	}))
	t.Cleanup(upstream.Close)
	return tokenSrv.URL, upstream.URL
}

func testGinContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c
}

func TestDatasourceCheckUsesCloudAuth(t *testing.T) {
	tokenURL, upstream := newOAuth2Servers(t, http.StatusOK)
	ds := models.Datasource{
		PluginType: models.PROMETHEUS,
		HTTPJson:   models.HTTP{Url: upstream},
		AuthJson: models.Auth{Config: dsauth.Config{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{
			ClientId: "c", ClientSecret: "s", TokenURL: tokenURL}}},
	}
	require.NoError(t, DatasourceCheck(testGinContext(), ds))
}

func TestDatasourceCheckExplainsAccessDenied(t *testing.T) {
	tokenURL, upstream := newOAuth2Servers(t, http.StatusForbidden)
	ds := models.Datasource{
		PluginType: models.PROMETHEUS,
		HTTPJson:   models.HTTP{Url: upstream},
		AuthJson: models.Auth{Config: dsauth.Config{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{
			ClientId: "c", ClientSecret: "s", TokenURL: tokenURL}}},
	}
	err := DatasourceCheck(testGinContext(), ds)
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "["+dsauth.CodeAccessDenied+"]"), err.Error())
}

func TestDatasourceCheckExplainsTokenFailure(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(tokenSrv.Close)
	ds := models.Datasource{
		PluginType: models.PROMETHEUS,
		HTTPJson:   models.HTTP{Url: "http://127.0.0.1:1"},
		AuthJson: models.Auth{Config: dsauth.Config{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{
			ClientId: "c", ClientSecret: "s", TokenURL: tokenSrv.URL}}},
	}
	err := DatasourceCheck(testGinContext(), ds)
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "["+dsauth.CodeTokenFailed+"]"), err.Error())
}

func TestDatasourceAuthPolicyGet(t *testing.T) {
	defer dsauth.SetPolicy(dsauth.Policy{})
	dsauth.SetPolicy(dsauth.Policy{DisableAmbientCredentials: true})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	(&Router{}).datasourceAuthPolicyGet(c)

	body := w.Body.String()
	require.Contains(t, body, `"allow_ambient_credentials":false`)
	require.Contains(t, body, `"allow_assume_role":true`)
	require.Contains(t, body, `"sigv4_allowed_services":["aps","es","aoss"]`)
	require.Contains(t, body, `https://prometheus.monitor.azure.com/.default`)
}

func TestCloseIdleConnectionsAcceptsWrappedTransport(t *testing.T) {
	rt, err := dsauth.Wrap(&http.Transport{}, dsauth.Config{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{
		ClientId: "c", ClientSecret: "s", TokenURL: "https://idp.example/token"}})
	require.NoError(t, err)
	transportPut(-1, 1, rt)
	_, has := transportGet(-1, 2) // updated_at changed → old transport closed, must not panic
	require.False(t, has)
}
