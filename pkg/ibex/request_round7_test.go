package ibex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGET_EncodesRepeatedQueryParametersAndBasicAuth(t *testing.T) {
	type request struct {
		method, path, user, pass, tenant string
		query                            url.Values
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		requests <- request{method: r.Method, path: r.URL.Path, user: user, pass: pass,
			tenant: r.Header.Get("X-Tenant"), query: r.URL.Query()}
		_, _ = w.Write([]byte(`{"id":42}`))
	}))
	t.Cleanup(server.Close)
	var out struct {
		ID int `json:"id"`
	}
	client := New(strings.TrimPrefix(server.URL, "http://"), "reader", "secret", 1000).
		Path("/tasks?existing=yes").Header("X-Tenant", "ops").
		QueryString("tag", "a & b").QueryString("tag", "c+d").Out(&out)

	err := client.GET()

	require.NoError(t, err)
	require.Equal(t, 42, out.ID)
	got := <-requests
	require.Equal(t, http.MethodGet, got.method)
	require.Equal(t, "/tasks", got.path)
	require.Equal(t, "reader", got.user)
	require.Equal(t, "secret", got.pass)
	require.Equal(t, "ops", got.tenant)
	require.Equal(t, url.Values{"existing": {"yes"}, "tag": {"a & b", "c+d"}}, got.query)
	require.Equal(t, time.Second, client.timeout)
}

func TestWriteMethods_SendJSONBody(t *testing.T) {
	cases := []struct {
		name string
		send func(*Ibex) error
	}{
		{name: "POST", send: (*Ibex).POST},
		{name: "PUT", send: (*Ibex).PUT},
		{name: "DELETE", send: (*Ibex).DELETE},
		{name: "PATCH", send: (*Ibex).PATCH},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			type request struct {
				method, contentType string
				body                map[string]string
				err                 error
			}
			requests := make(chan request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				err := json.NewDecoder(r.Body).Decode(&body)
				requests <- request{method: r.Method, contentType: r.Header.Get("Content-Type"), body: body, err: err}
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			t.Cleanup(server.Close)
			var out map[string]bool
			client := New(server.URL, "", "", 1000).Path("/task").In(map[string]string{"script": "echo hello"}).Out(&out)

			err := tc.send(client)

			require.NoError(t, err)
			require.True(t, out["ok"])
			got := <-requests
			require.NoError(t, got.err)
			require.Equal(t, tc.name, got.method)
			require.Equal(t, "application/json", got.contentType)
			require.Equal(t, map[string]string{"script": "echo hello"}, got.body)
		})
	}
}

func TestRequest_RejectsUnencodableInput(t *testing.T) {
	client := New("http://127.0.0.1", "", "", 1000).In(make(chan int))

	err := client.POST()

	require.ErrorContains(t, err, "unsupported type")
}

func TestRequest_RejectsMalformedURL(t *testing.T) {
	client := New("http://[invalid", "", "", 1000)

	err := client.GET()

	require.ErrorContains(t, err, "missing ']' in host")
}

func TestRequest_ReportsInvalidResponseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(server.Close)
	var out map[string]interface{}

	err := New(server.URL, "", "", 1000).Out(&out).GET()

	require.ErrorContains(t, err, "invalid character")
	require.Nil(t, out)
}

func TestRequest_PropagatesTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(server.Close)
	var out interface{}

	err := New(server.URL, "", "", 1000).Out(&out).POST()

	require.Error(t, err)
	require.Nil(t, out)
}
