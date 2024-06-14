package poster

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func TestPostByUrls(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DataResponse[interface{}]{Dat: "", Err: ""}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx := &ctx.Context{
		CenterApi: conf.CenterApi{
			Addrs: []string{server.URL},
		}}

	if err := PostByUrls(ctx, "/v1/n9e/server-heartbeat", map[string]string{"a": "aa"}); err != nil {
		t.Errorf("PostByUrls() error = %v ", err)
	}
}

func TestPostByUrlsWithResp(t *testing.T) {

	expected := int64(123)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := DataResponse[int64]{Dat: expected, Err: ""}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	ctx := &ctx.Context{
		CenterApi: conf.CenterApi{
			Addrs: []string{server.URL},
		}}

	gotT, err := PostByUrlsWithResp[int64](ctx, "/v1/n9e/event-persist", map[string]string{"b": "bb"})
	if err != nil {
		t.Errorf("PostByUrlsWithResp() error = %v", err)
		return
	}
	if gotT != expected {
		t.Errorf("PostByUrlsWithResp() gotT = %v,expected = %v", gotT, expected)
	}

}
