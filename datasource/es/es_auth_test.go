package es

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/pkg/dsauth"

	"github.com/olivere/elastic/v7"
)

// SigV4 for Amazon OpenSearch Service: the client must sign for service "es"
// and must not send the leftover Basic credentials from settings.
func TestElasticsearchSigV4ViaApplyAuth(t *testing.T) {
	var authz []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz = append(authz, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":{"number":"7.10.2"}}`))
	}))
	defer srv.Close()

	ds, err := datasource.GetDatasourceByType(ESType, map[string]interface{}{
		"es.nodes": []string{srv.URL},
		"es.basic": map[string]interface{}{"es.user": "u", "es.password": "p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	datasource.ApplyAuth(ds, dsauth.Config{Type: dsauth.TypeSigV4, SigV4: &dsauth.SigV4Config{
		Region: "us-east-1", Service: "es", CredentialType: dsauth.AWSCredentialKeys, AccessKey: "AKID", SecretKey: "s",
	}})
	if err := ds.InitClient(); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.(*Elasticsearch).Client.PerformRequest(context.Background(), elastic.PerformRequestOptions{Method: "GET", Path: "/"}); err != nil {
		t.Fatal(err)
	}

	if len(authz) == 0 {
		t.Fatal("no request reached the server")
	}
	for _, a := range authz {
		if !strings.HasPrefix(a, "AWS4-HMAC-SHA256 ") || !strings.Contains(a, "/us-east-1/es/aws4_request") {
			t.Fatalf("Authorization = %q", a)
		}
	}
}
