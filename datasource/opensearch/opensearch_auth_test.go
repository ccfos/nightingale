package opensearch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/pkg/dsauth"
)

func TestOpenSearchSigV4ViaApplyAuth(t *testing.T) {
	var authz []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz = append(authz, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":{"number":"2.11.0","distribution":"opensearch"}}`))
	}))
	defer srv.Close()

	settings := map[string]interface{}{
		"os.nodes": []string{srv.URL},
		"os.basic": map[string]interface{}{"os.user": "u", "os.password": "p"},
	}
	ds, err := datasource.GetDatasourceByType(OpenSearchType, settings)
	if err != nil {
		t.Fatal(err)
	}
	datasource.ApplyAuth(ds, dsauth.Config{Type: dsauth.TypeSigV4, SigV4: &dsauth.SigV4Config{
		Region: "eu-west-1", Service: "aoss", CredentialType: dsauth.AWSCredentialKeys, AccessKey: "AKID", SecretKey: "s",
	}})

	plain, _ := datasource.GetDatasourceByType(OpenSearchType, settings)
	if ds.Equal(plain) {
		t.Fatal("a changed auth must make Equal false so the client is rebuilt")
	}

	if err := ds.InitClient(); err != nil {
		t.Fatal(err)
	}
	resp, err := ds.(*OpenSearch).Client.Info()
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if len(authz) != 1 || !strings.Contains(authz[0], "/eu-west-1/aoss/aws4_request") {
		t.Fatalf("Authorization = %q", authz)
	}
}
