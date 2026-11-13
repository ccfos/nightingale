package prom

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/api"
	"github.com/stretchr/testify/require"
)

// A successful remote-write fallback must not report the preceding endpoint's failure.
func TestWriterPost_SucceedsOnFailoverURL(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		if r.URL.Path == "/failing" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := api.NewClient(api.Config{Address: server.URL})
	require.NoError(t, err)
	writer := NewWriter(client, ClientOptions{Url: server.URL + "/failing," + server.URL + "/ok"})

	err = writer.Post([]byte("remote-write payload"))

	require.NoError(t, err)
	require.Len(t, requests, 2)
	require.Equal(t, "/failing", <-requests)
	require.Equal(t, "/ok", <-requests)
}
