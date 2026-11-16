package prom

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReaderClientRetiresIdleConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the query client's idle timeout")
	}
	closed := make(chan struct{}, 4)
	addresses := make(chan string, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addresses <- r.RemoteAddr
		w.WriteHeader(http.StatusOK)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	client, err := new(PromClientMap).newReaderClientFromPromOption(PromOption{
		Url: server.URL, Timeout: 1000, DialTimeout: 1000, MaxIdleConnsPerHost: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	query := func() string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/query", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, _, err := client.Do(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		return <-addresses
	}
	first := query()
	if next := query(); next != first {
		t.Fatal("connection was not reused before the idle timeout")
	}
	select {
	case <-closed:
	case <-time.After(40 * time.Second):
		t.Fatal("client did not retire its idle connection before a one-minute server timeout")
	}
	if next := query(); next == first {
		t.Fatal("query reused the retired connection")
	}
}
