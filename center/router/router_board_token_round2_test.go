package router

import (
	"net/http"
	"testing"
	"time"

	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBoardTokenDetect_SkipsAuthenticationOnlyForValidBoardTokens(t *testing.T) {
	cases := []struct {
		name  string
		token models.SourceToken
		valid bool
	}{
		{name: "valid", token: models.SourceToken{SourceType: "board", SourceId: "7"}, valid: true},
		{name: "expired", token: models.SourceToken{SourceType: "board", SourceId: "7", ExpireAt: 1}},
		{name: "wrong source type", token: models.SourceToken{SourceType: "event", SourceId: "7"}},
		{name: "invalid source id", token: models.SourceToken{SourceType: "board", SourceId: "bad"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.SourceToken{})
			tc.token.Token = "share"
			require.NoError(t, rt.Ctx.DB.Create(&tc.token).Error)
			authCalled := false
			handler := func(c *gin.Context) {
				rt.boardTokenDetect()(c)
				skipIfBoardToken(func(c *gin.Context) { authCalled = true })(c)
				bid, ok := BoardTokenBid(c)
				c.JSON(http.StatusOK, gin.H{"id": bid, "share": ok})
			}

			w := identityRequest(t, rt, handler, nil, "1?__token=share", nil)

			require.Equal(t, !tc.valid, authCalled)
			if tc.valid {
				require.JSONEq(t, `{"id":7,"share":true}`, w.Body.String())
			} else {
				require.JSONEq(t, `{"id":0,"share":false}`, w.Body.String())
			}
		})
	}
}

func TestBoardTokenQueryContext_RestrictsDatasourceAndEnforcesReadOnly(t *testing.T) {
	rt := identityRouter(t, &models.BoardPayload{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	rt.DatasourceCache.Set(map[int64]*models.Datasource{
		9: {Id: 9, PluginType: "mysql"}, 10: {Id: 10, PluginType: "mysql"},
	}, 2, 1)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 7,
		`{"var":[{"type":"datasource","definition":"mysql"}],"panels":[{"datasourceValue":8}]}`))
	boardDsSetMu.Lock()
	original := boardDsSetItems
	boardDsSetItems = make(map[int64]boardDsSetItem)
	boardDsSetMu.Unlock()
	t.Cleanup(func() { boardDsSetMu.Lock(); boardDsSetItems = original; boardDsSetMu.Unlock() })
	cases := []struct {
		name       string
		datasource int64
		shared     bool
		status     int
	}{
		{name: "literal datasource", datasource: 8, shared: true, status: 200},
		{name: "variable datasource", datasource: 9, shared: true, status: 200},
		{name: "foreign datasource", datasource: 99, shared: true, status: 403},
		{name: "ordinary request", datasource: 99, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(c *gin.Context) {
				if tc.shared {
					c.Set(boardTokenBidKey, int64(7))
				}
				shared := rt.boardTokenQueryContext(c, tc.datasource)
				cc, _ := dskittypes.CallContextFromCtx(c.Request.Context())
				c.JSON(http.StatusOK, gin.H{"shared": shared, "readonly": cc.EnforceReadOnly})
			}

			w := identityRequest(t, rt, handler, nil, "1", nil)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusForbidden {
				require.Contains(t, w.Body.String(), "datasource is not referenced")
			} else if tc.shared {
				require.JSONEq(t, `{"shared":true,"readonly":true}`, w.Body.String())
			} else {
				require.JSONEq(t, `{"shared":false,"readonly":false}`, w.Body.String())
			}
		})
	}
}

func TestBoardTokenPermission_ExpiredCacheRechecksUpdatedBoardPayload(t *testing.T) {
	rt := identityRouter(t, &models.BoardPayload{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 7, `{"panels":[{"datasourceValue":9}]}`))
	boardDsSetMu.Lock()
	original := boardDsSetItems
	boardDsSetItems = map[int64]boardDsSetItem{
		7: {set: map[int64]struct{}{8: {}}, expireAt: time.Unix(1, 0)},
	}
	boardDsSetMu.Unlock()
	t.Cleanup(func() { boardDsSetMu.Lock(); boardDsSetItems = original; boardDsSetMu.Unlock() })
	handler := func(c *gin.Context) {
		c.Set(boardTokenBidKey, int64(7))
		rt.checkBoardTokenDsPerm(c, 8)
		c.String(http.StatusOK, "allowed")
	}

	w := identityRequest(t, rt, handler, nil, "1", nil)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "datasource is not referenced")
}
