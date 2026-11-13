package iotdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/apache/iotdb-client-go/v2/client"
)

func TestRPCEndpoint(t *testing.T) {
	tests := []struct {
		name, addr, rest, wantHost, wantPort string
	}{
		{name: "explicit host port", addr: "192.168.1.2:7777", wantHost: "192.168.1.2", wantPort: "7777"},
		{name: "explicit url", addr: "http://iotdb.example:7777", wantHost: "iotdb.example", wantPort: "7777"},
		{name: "explicit host", addr: "iotdb.example", wantHost: "iotdb.example", wantPort: "6667"},
		{name: "derived from url", rest: "http://iotdb.example:18080", wantHost: "iotdb.example", wantPort: "6667"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, err := (&Iotdb{RPCAddr: tt.addr, Addr: tt.rest}).rpcEndpoint()
			if err != nil {
				t.Fatal(err)
			}
			if gotHost != tt.wantHost || gotPort != tt.wantPort {
				t.Fatalf("endpoint=%s:%s, want %s:%s", gotHost, gotPort, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestRPCEndpointRequiresAddress(t *testing.T) {
	_, _, err := (&Iotdb{}).rpcEndpoint()
	if err == nil || !strings.Contains(err.Error(), "iotdb.rpc_addr") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRPCEndpointRejectsMalformedAddress(t *testing.T) {
	for _, addr := range []string{"http://", ":6667", "host:", "host/path"} {
		t.Run(addr, func(t *testing.T) {
			if _, _, err := (&Iotdb{RPCAddr: addr}).rpcEndpoint(); err == nil {
				t.Fatalf("expected malformed RPC address %q to fail", addr)
			}
		})
	}
}

func TestRPCEndpointSupportsIPv6(t *testing.T) {
	host, port, err := (&Iotdb{RPCAddr: "[2001:db8::1]:7777"}).rpcEndpoint()
	if err != nil {
		t.Fatal(err)
	}
	if host != "2001:db8::1" || port != "7777" {
		t.Fatalf("endpoint=%s:%s, want 2001:db8::1:7777", host, port)
	}
}

func TestQuoteIdentifier(t *testing.T) {
	if got := quoteIdentifier(`db"name`); got != `"db""name"` {
		t.Fatalf("quoted identifier=%q", got)
	}
}

func TestQueryTimeoutUsesContextDeadline(t *testing.T) {
	it := &Iotdb{Timeout: 30000}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	got := it.queryTimeoutMs(ctx)
	if got <= 0 || got > 40 {
		t.Fatalf("timeout=%dms, want positive value <= 40ms", got)
	}
}

func TestRESTQueryErrorPreservesServerMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":701,"message":"invalid SQL"}`))
	}))
	defer server.Close()

	it := &Iotdb{Addr: server.URL, Timeout: 1000}
	_, err := it.queryREST(context.Background(), "metrics", "select", 0)
	if err == nil || !strings.Contains(err.Error(), "invalid SQL") {
		t.Fatalf("expected server error message, got %v", err)
	}
}

func TestRPCPoolCreationDoesNotLogPassword(t *testing.T) {
	it := &Iotdb{RPCAddr: "127.0.0.1:6667", Basic: &IotdbBasicAuth{User: "root", Password: "super-secret"}}
	if got := fmt.Sprintf("%+v", it); strings.Contains(got, "super-secret") {
		t.Fatalf("password leaked in formatted datasource: %s", got)
	}
}

func TestIotdbStringOmitsPassword(t *testing.T) {
	it := &Iotdb{RPCAddr: "127.0.0.1:6667", Basic: &IotdbBasicAuth{User: "root", Password: "super-secret"}}
	if got := it.String(); strings.Contains(got, "super-secret") {
		t.Fatalf("password leaked in datasource string: %s", got)
	}
}

type fakeTableSession struct {
	calls     []string
	useErr    error
	queryErr  error
	nilResult bool
}

func (f *fakeTableSession) Insert(_ *client.Tablet) error { return nil }

func (f *fakeTableSession) ExecuteNonQueryStatement(sql string) error {
	f.calls = append(f.calls, "nonquery:"+sql)
	return f.useErr
}

func (f *fakeTableSession) ExecuteQueryStatement(sql string, _ *int64) (*client.SessionDataSet, error) {
	f.calls = append(f.calls, "query:"+sql)
	if f.nilResult {
		return nil, nil
	}
	return &client.SessionDataSet{}, f.queryErr
}

func (f *fakeTableSession) Close() error { return nil }

func TestQueryOnSessionUsesDatabaseBeforeQuery(t *testing.T) {
	session := &fakeTableSession{}
	if _, err := queryOnSession(session, "metrics", "SELECT 1", 1000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{`nonquery:USE "metrics"`, "query:SELECT 1"}
	if !reflect.DeepEqual(session.calls, want) {
		t.Fatalf("calls=%v, want %v", session.calls, want)
	}
}

func TestQueryOnSessionQuotesDatabaseIdentifier(t *testing.T) {
	session := &fakeTableSession{}
	if _, err := queryOnSession(session, `db"name`, "SELECT 1", 1000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := session.calls[0]; got != `nonquery:USE "db""name"` {
		t.Fatalf("USE statement=%q, want quoted identifier", got)
	}
}

func TestQueryOnSessionSurfacesUseError(t *testing.T) {
	session := &fakeTableSession{useErr: errors.New("database not found")}
	_, err := queryOnSession(session, "missing", "SELECT 1", 1000)
	if err == nil || !strings.Contains(err.Error(), "cannot USE database") {
		t.Fatalf("expected USE error to surface, got %v", err)
	}
	if len(session.calls) != 1 {
		t.Fatalf("query must not run after a USE error, calls=%v", session.calls)
	}
}

func TestQueryOnSessionSurfacesQueryError(t *testing.T) {
	session := &fakeTableSession{queryErr: errors.New("syntax error")}
	_, err := queryOnSession(session, "metrics", "SELECT bad", 1000)
	if err == nil || !strings.Contains(err.Error(), "IoTDB RPC query failed") {
		t.Fatalf("expected query error to surface, got %v", err)
	}
}

func TestQueryOnSessionRejectsNilResult(t *testing.T) {
	session := &fakeTableSession{nilResult: true}
	if _, err := queryOnSession(session, "metrics", "SELECT 1", 1000); err == nil {
		t.Fatal("expected a nil result set to be rejected")
	}
}

func TestTablePoolIsReused(t *testing.T) {
	it := &Iotdb{RPCAddr: "127.0.0.1:6667"}
	defer it.Close()

	p1, err := it.tablePool()
	if err != nil {
		t.Fatal(err)
	}
	p2, err := it.tablePool()
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Fatal("table pool was not reused")
	}
}

func TestBeginQueryRejectsClosedDatasource(t *testing.T) {
	it := &Iotdb{RPCAddr: "127.0.0.1:6667"}
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}
	if release, err := it.beginQuery(); err == nil {
		release()
		t.Fatal("expected a closed datasource to reject queries")
	}
}
