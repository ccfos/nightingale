package loki

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/pkg/dsauth"
)

// The cloud auth reaches the plugin through datasource.ApplyAuth (dscache and
// datasource upsert), not through settings.
func TestLokiCloudAuthViaApplyAuth(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"loki-tok","token_type":"bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer loki-tok" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":["app"]}`))
	}))
	defer server.Close()

	ds, err := datasource.GetDatasourceByType(LokiType, map[string]interface{}{
		"loki.addr":  server.URL,
		"loki.basic": map[string]interface{}{"loki.user": "u", "loki.password": "p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	auth := dsauth.Config{Type: dsauth.TypeOAuth2, OAuth2: &dsauth.OAuth2Config{ClientId: "c", ClientSecret: "s", TokenURL: tokenSrv.URL}}
	datasource.ApplyAuth(ds, auth)

	plain, _ := datasource.GetDatasourceByType(LokiType, map[string]interface{}{
		"loki.addr":  server.URL,
		"loki.basic": map[string]interface{}{"loki.user": "u", "loki.password": "p"},
	})
	if ds.Equal(plain) {
		t.Fatal("a changed auth must make Equal false so the client is rebuilt")
	}

	if err := ds.InitClient(); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.(*Loki).LabelNames(context.Background(), "", 0, 0, "", 10); err != nil {
		t.Fatal(err)
	}
}
