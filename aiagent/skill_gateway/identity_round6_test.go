package skillgateway

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func gatewayDBRound6(t *testing.T, tables ...interface{}) *ctxpkg.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	if len(tables) > 0 {
		require.NoError(t, db.AutoMigrate(tables...))
	}
	return ctxpkg.NewContext(context.Background(), db, true)
}

func TestResolveUserToken_ReusesOnlyBoundUsersToken(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.UserToken{})
	require.NoError(t, models.DB(dbctx).Create(&[]models.UserToken{
		{Username: "other", Token: "other-token"},
		{Username: "alice", Token: "alice-token"},
	}).Error)
	called := false

	token, err := resolveUserToken(dbctx, &models.User{Username: "alice"}, func(string, *models.User) { called = true })

	require.NoError(t, err)
	require.Equal(t, "alice-token", token)
	require.False(t, called, "an existing token is already available in the authentication cache")
	var count int64
	require.NoError(t, models.DB(dbctx).Model(&models.UserToken{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestResolveUserToken_PersistsAndCachesFreshToken(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.UserToken{})
	user := &models.User{Id: 7, Username: "alice"}
	var cachedToken string
	var cachedUser *models.User

	token, err := resolveUserToken(dbctx, user, func(token string, user *models.User) {
		cachedToken, cachedUser = token, user
	})

	require.NoError(t, err)
	decoded, err := hex.DecodeString(token)
	require.NoError(t, err)
	require.Len(t, decoded, 20)
	require.Equal(t, token, cachedToken)
	require.Same(t, user, cachedUser)
	rows, err := models.GetTokensByUsername(dbctx, "alice")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, token, rows[0].Token)
	require.Equal(t, "skill-gateway", rows[0].TokenName)
}

func TestResolveUserToken_CreationWorksWithoutCacheCallback(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.UserToken{})

	token, err := resolveUserToken(dbctx, &models.User{Username: "alice"}, nil)

	require.NoError(t, err)
	rows, err := models.GetTokensByUsername(dbctx, "alice")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, token, rows[0].Token)
	require.NotEmpty(t, token)
}

func TestResolveUserToken_PropagatesReadFailure(t *testing.T) {
	dbctx := gatewayDBRound6(t)
	called := false

	token, err := resolveUserToken(dbctx, &models.User{Username: "alice"}, func(string, *models.User) { called = true })

	require.ErrorContains(t, err, "user_token")
	require.Empty(t, token)
	require.False(t, called)
}

func TestResolveUserToken_DoesNotCacheFailedInsert(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.UserToken{})
	require.NoError(t, models.DB(dbctx).Exec(`CREATE TRIGGER reject_token BEFORE INSERT ON user_token
 BEGIN SELECT RAISE(ABORT, 'token writes disabled'); END`).Error)
	called := false

	token, err := resolveUserToken(dbctx, &models.User{Username: "alice"}, func(string, *models.User) { called = true })

	require.ErrorContains(t, err, "token writes disabled")
	require.Empty(t, token)
	require.False(t, called)
	rows, err := models.GetTokensByUsername(dbctx, "alice")
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestStart_RejectsMissingIdentityOrConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		params Params
		want   string
	}{
		{name: "nil database", params: Params{}, want: "nil db context"},
		{name: "unbound user", params: Params{DBCtx: &ctxpkg.Context{}}, want: "no bound user id"},
		{
			name:   "blank upstream",
			params: Params{DBCtx: &ctxpkg.Context{}, UserID: 7, Config: Config{BaseURL: "  "}},
			want:   "no n9e api base url",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "gw.sock")

			gateway, err := Start(socket, tc.params)

			require.Nil(t, gateway)
			require.ErrorContains(t, err, tc.want)
			require.NoFileExists(t, socket)
		})
	}
}

func TestStart_RejectsUnknownBoundUser(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.User{})

	gateway, err := Start(filepath.Join(t.TempDir(), "gw.sock"), Params{
		DBCtx: dbctx, UserID: 7, Config: Config{BaseURL: "http://127.0.0.1"},
	})

	require.Nil(t, gateway)
	require.ErrorContains(t, err, "bound user 7 not found")
}

func TestStart_PropagatesUserLookupFailure(t *testing.T) {
	dbctx := gatewayDBRound6(t)

	gateway, err := Start(filepath.Join(t.TempDir(), "gw.sock"), Params{
		DBCtx: dbctx, UserID: 7, Config: Config{BaseURL: "http://127.0.0.1"},
	})

	require.Nil(t, gateway)
	require.ErrorContains(t, err, "resolve user 7")
}

func TestStart_PropagatesTokenLookupFailure(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.User{})
	require.NoError(t, models.DB(dbctx).Create(&models.User{Id: 7, Username: "alice"}).Error)

	gateway, err := Start(filepath.Join(t.TempDir(), "gw.sock"), Params{
		DBCtx: dbctx, UserID: 7, Config: Config{BaseURL: "http://127.0.0.1"},
	})

	require.Nil(t, gateway)
	require.ErrorContains(t, err, "resolve token for alice")
}

func TestStart_ReportsSocketBindFailure(t *testing.T) {
	dbctx := gatewayDBRound6(t, &models.User{}, &models.UserToken{})
	require.NoError(t, models.DB(dbctx).Create(&models.User{Id: 7, Username: "alice"}).Error)
	require.NoError(t, models.DB(dbctx).Create(&models.UserToken{Username: "alice", Token: "existing"}).Error)
	socket := filepath.Join(t.TempDir(), "missing", "gw.sock")

	gateway, err := Start(socket, Params{
		DBCtx: dbctx, UserID: 7, Config: Config{BaseURL: "http://127.0.0.1"},
	})

	require.Nil(t, gateway)
	require.ErrorContains(t, err, "listen "+socket)
	require.NoFileExists(t, socket)
}

// A listener stub checks cleanup without starting an accept loop.
type closeListenerRound6 struct{ closed int }

func (l *closeListenerRound6) Accept() (net.Conn, error) { return nil, errors.New("unused") }
func (l *closeListenerRound6) Close() error              { l.closed++; return nil }
func (l *closeListenerRound6) Addr() net.Addr            { return nil }

func TestGatewayClose_RemovesSocketAndClosesListenerOnce(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "gw.sock")
	require.NoError(t, os.WriteFile(socket, nil, 0o600))
	listener := &closeListenerRound6{}
	gateway := &Gateway{path: socket, ln: listener}
	require.Equal(t, socket, gateway.SocketPath())

	gateway.Close()
	gateway.Close()
	(*Gateway)(nil).Close()

	require.Equal(t, 1, listener.closed)
	require.True(t, gateway.closed.Load())
	require.NoFileExists(t, socket)
}

func TestHandleRequest_RejectsMalformedJSON(t *testing.T) {
	gateway := testGateway("", nil)

	resp := gateway.handleRequest([]byte(`{"method":`))

	require.False(t, resp.OK)
	require.Zero(t, resp.Status)
	require.Contains(t, resp.Error, "invalid request json")
}

func TestHandleRequest_DefaultsMethodAndEncodesQuery(t *testing.T) {
	requests := make(chan *http.Request, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		_, _ = w.Write([]byte("plain response"))
	}))
	t.Cleanup(upstream.Close)
	gateway := testGateway(upstream.URL, nil)
	gateway.tokenHeader = "X-Custom-Token"

	resp := gateway.handleRequest([]byte(`{"path":" /api/n9e/targets?ignored=1 ","query":{"q":"a & b","group":"2"}}`))

	require.True(t, resp.OK)
	require.Equal(t, "plain response", resp.Data)
	req := <-requests
	require.Equal(t, http.MethodGet, req.Method)
	require.Equal(t, "/api/n9e/targets", req.URL.Path)
	require.Equal(t, "a & b", req.URL.Query().Get("q"))
	require.Equal(t, "2", req.URL.Query().Get("group"))
	require.Empty(t, req.URL.Query().Get("ignored"))
	require.Equal(t, "tok-123", req.Header.Get("X-Custom-Token"))
	require.Empty(t, req.Header.Get(defaultTokenHeader))
	require.Equal(t, "en", req.Header.Get("X-Language"))
	require.Empty(t, req.Header.Get("Content-Type"))
}

func TestProxy_ReportsInvalidUpstreamURL(t *testing.T) {
	gateway := testGateway("://invalid", nil)

	resp := gateway.proxy(http.MethodGet, "/targets", nil, nil)

	require.False(t, resp.OK)
	require.Contains(t, resp.Error, "build request:")
}

type failingTransportRound6 struct{}

func (failingTransportRound6) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("upstream unavailable")
}

func TestProxy_ReportsTransportFailure(t *testing.T) {
	gateway := testGateway("http://127.0.0.1", nil)
	gateway.client = &http.Client{Transport: failingTransportRound6{}}

	resp := gateway.proxy(http.MethodGet, "/targets", nil, nil)

	require.False(t, resp.OK)
	require.Contains(t, resp.Error, "upstream call failed:")
	require.Contains(t, resp.Error, "upstream unavailable")
}

func TestMergeDenyPaths_NormalizesOperatorPrefixes(t *testing.T) {
	gateway := testGateway("", []string{"  SeCrEt  ", "", "  ", "/PRIVATE"})

	require.ErrorContains(t, gateway.checkAllowed(http.MethodGet, "/secret/details"), "blacklisted")
	require.ErrorContains(t, gateway.checkAllowed(http.MethodGet, "/PRIVATE/details"), "blacklisted")
	require.ErrorContains(t, gateway.checkAllowed(http.MethodGet, "/users"), "blacklisted")
	require.NoError(t, gateway.checkAllowed(http.MethodGet, "/targets"))
}
