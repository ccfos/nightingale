package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatasourceRedactSecretsURL(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"empty", "", ""},
		{"plain endpoint", "http://prom.example:9090/prometheus", "http://prom.example:9090/prometheus"},
		{"credentials", "http://url-user:url-password@prom.example:9090/prometheus", "http://prom.example:9090/prometheus"},
		{"https credentials", "https://url-user:url-password@prom.example/prometheus", "https://prom.example/prometheus"},
		{"username only", "https://url-token@prom.example", "https://prom.example"},
		{"empty password", "http://url-user:@prom.example", "http://prom.example"},
		{"encoded credentials", "http://url%40user:p%40ss%3Aword@prom.example/a%2Fb", "http://prom.example/a%2Fb"},
		{"multiple at signs", "http://url-user:p@ss@prom.example", "http://prom.example"},
		{"ipv6", "http://url-user:url-password@[::1]:9090/prometheus", "http://[::1]:9090/prometheus"},
		{"query and fragment", "https://url-user:url-password@prom.example/path?timeout=10#section", "https://prom.example/path?timeout=10#section"},
		{"invalid escape", "http://url-user:p%zz@prom.example", ""},
		{"invalid host", "http://url-user:url-password@[::1", ""},
		{"relative URL", "//url-user:url-password@prom.example", ""},
		{"missing host", "http:///url-user:url-password@prom.example", ""},
		{"opaque URL", "http:url-user:url-password@prom.example", ""},
		{"other scheme", "ftp://url-user:url-password@prom.example", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := Datasource{HTTPJson: HTTP{Url: tc.raw, Urls: []string{tc.raw}}}
			ds.RedactSecrets()
			require.Equal(t, tc.want, ds.HTTPJson.Url)
			require.Equal(t, []string{tc.want}, ds.HTTPJson.Urls)
			payload, err := json.Marshal(ds)
			require.NoError(t, err)
			for _, secret := range []string{"url-user", "url-password", "url-token", "url%40user", "p%40ss%3Aword"} {
				require.NotContains(t, string(payload), secret)
			}
		})
	}
}

func TestDatasourceRedactSecretsPreservesOriginal(t *testing.T) {
	authReceived := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ := r.BasicAuth()
		authReceived <- user + ":" + password
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	rawURL := strings.Replace(server.URL, "://", "://url-user:url-password@", 1)
	original := Datasource{
		HTTPJson: HTTP{
			Url: rawURL, Urls: []string{rawURL, "https://second-user:second-password@es.example:9200"},
			Headers: map[string]string{"Authorization": "header-secret"},
			TLS:     TLS{CACert: "ca-secret", ClientCert: "cert-secret", ClientKey: "key-secret", ClientKeyPassword: "key-password"},
		},
		AuthJson:        Auth{BasicAuth: true, BasicAuthUser: "auth-user", BasicAuthPassword: "auth-password"},
		SettingsJson:    map[string]interface{}{"password": "settings-secret"},
		SettingsEncoded: "settings-encoded-secret",
		AuthEncoded:     "auth-encoded-secret",
	}
	require.NoError(t, original.FE2DB())
	response := original // The HTTP URL slice still shares its backing array.
	response.RedactSecrets()
	require.Equal(t, server.URL, response.HTTPJson.Url)
	require.Equal(t, []string{server.URL, "https://es.example:9200"}, response.HTTPJson.Urls)
	require.Equal(t, []string{rawURL, "https://second-user:second-password@es.example:9200"}, original.HTTPJson.Urls)
	require.Equal(t, rawURL, original.HTTPJson.Url)
	require.Empty(t, response.HTTP)
	require.Empty(t, response.Auth)
	require.Empty(t, response.Settings)
	payload, err := json.Marshal(response)
	require.NoError(t, err)
	for _, secret := range []string{"url-user", "url-password", "second-user", "second-password", "header-secret", "ca-secret", "cert-secret", "key-secret", "key-password", "auth-user", "auth-password", "settings-secret", "settings-encoded-secret", "auth-encoded-secret"} {
		require.NotContains(t, string(payload), secret)
	}

	// The original configuration still authenticates downstream after redaction.
	var selectedURL string
	req, err := (HTTP{Url: original.HTTPJson.Url}).NewReq(&selectedURL)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "url-user:url-password", <-authReceived)
}

func TestDatasourceRedactSecretsURLSlices(t *testing.T) {
	for _, urls := range [][]string{nil, {}} {
		ds := Datasource{HTTPJson: HTTP{Urls: urls}}
		ds.RedactSecrets()
		require.Equal(t, urls, ds.HTTPJson.Urls)
	}

	// Legacy Elasticsearch configurations populate Urls from Url on read.
	ds := Datasource{PluginType: ELASTICSEARCH, HTTPJson: HTTP{Url: "http://url-user:url-password@es.example:9200"}}
	require.NoError(t, ds.FE2DB())
	loaded := Datasource{PluginType: ds.PluginType, HTTP: ds.HTTP}
	require.NoError(t, loaded.DB2FE())
	loaded.RedactSecrets()
	require.Equal(t, "http://es.example:9200", loaded.HTTPJson.Url)
	require.Equal(t, []string{"http://es.example:9200"}, loaded.HTTPJson.Urls)
}
