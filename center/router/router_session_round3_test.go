package router

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func sessionRouter(t *testing.T) (*Router, *miniredis.Miniredis) {
	t.Helper()
	rt := identityRouter(t, &models.User{}, &models.Configs{})
	seedIdentitySalt(t, rt)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	rt.Redis = client
	rt.HTTP.JWTAuth = httpx.JWTAuth{
		SigningKey: "round-three-session-key", AccessExpired: 10, RefreshExpired: 60, RedisKeyPrefix: "test-jwt:",
	}
	rt.Sso = &sso.SsoClient{LDAP: ldapx.New(ldapx.Config{})}
	return rt, server
}

func TestLoginPost_LocalCredentialsCreateVerifiableSession(t *testing.T) {
	rt, server := sessionRouter(t)
	hash, err := models.CryptoPass(rt.Ctx, "secret")
	require.NoError(t, err)
	user := models.User{Id: 1, Username: "alice", Password: hash, Roles: "Standard", Contacts: []byte("{}")}
	require.NoError(t, rt.Ctx.DB.Create(&user).Error)

	w := identityRequest(t, rt, rt.loginPost, loginForm{Username: "alice", Password: "secret"}, "0", nil)

	var got struct {
		User         models.User `json:"user"`
		AccessToken  string      `json:"access_token"`
		RefreshToken string      `json:"refresh_token"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "alice", got.User.Username)
	require.Empty(t, got.User.Password)
	require.NotEmpty(t, got.RefreshToken)
	token, err := jwt.Parse(got.AccessToken, func(token *jwt.Token) (interface{}, error) {
		require.Equal(t, jwt.SigningMethodHS256, token.Method)
		return []byte(rt.HTTP.JWTAuth.SigningKey), nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)
	claims := token.Claims.(jwt.MapClaims)
	require.Equal(t, "1-alice", claims["user_identity"])
	session, err := server.Get(rt.wrapJwtKey(claims["access_uuid"].(string)))
	require.NoError(t, err)
	require.Equal(t, "1-alice", session)
}

func TestLoginPost_InvalidCredentialsDoNotCreateSession(t *testing.T) {
	rt, server := sessionRouter(t)
	hash, err := models.CryptoPass(rt.Ctx, "secret")
	require.NoError(t, err)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Password: hash}).Error)

	w := identityRequest(t, rt, rt.loginPost, loginForm{Username: "alice", Password: "wrong"}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "Username or password invalid")
	require.Empty(t, server.Keys())
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, hash, saved.Password)
}

func TestLoginPost_CaptchaRejectionStopsBeforeCredentialCheck(t *testing.T) {
	rt, server := sessionRouter(t)
	original := captchaStore
	captchaStore = &CaptchaRedisStore{redis: rt.Redis}
	t.Cleanup(func() { captchaStore = original })
	require.NoError(t, captchaStore.Set("challenge", "12"))
	rt.HTTP.ShowCaptcha.Enable = true

	w := identityRequest(t, rt, rt.loginPost,
		loginForm{Username: "alice", Password: "secret", Captchaid: "challenge", Verifyvalue: "13"}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "incorrect verification code")
	require.Empty(t, server.Keys())
}

func TestLogoutPost_DeletesBothTokensAndIdentityToken(t *testing.T) {
	rt, server := sessionRouter(t)
	tokens, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, "1-alice")
	require.NoError(t, err)
	require.NoError(t, rt.createAuth(context.Background(), "1-alice", tokens))
	require.NoError(t, rt.saveIdToken(context.Background(), 1, "identity-token"))
	handler := func(c *gin.Context) {
		c.Request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		rt.logoutPost(c)
	}

	w := identityRequest(t, rt, handler, nil, "0", &models.User{Id: 1, Username: "alice"})

	var destination string
	require.Empty(t, identityReply(t, w, &destination))
	require.Empty(t, destination)
	require.Empty(t, server.Keys())
}

func TestLogoutPost_RejectsInvalidTokenWithoutDeletingSession(t *testing.T) {
	rt, server := sessionRouter(t)
	require.NoError(t, server.Set("existing", "keep"))

	w := identityRequest(t, rt, rt.logoutPost, nil, "0", &models.User{Id: 1})

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "failed to parse jwt token")
	require.Equal(t, []string{"existing"}, server.Keys())
}

func TestCreateAuth_SingleLoginRevokesPreviousSession(t *testing.T) {
	rt, server := sessionRouter(t)
	rt.HTTP.JWTAuth.SingleLogin = true
	oldTokens, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, "1-alice")
	require.NoError(t, err)
	require.NoError(t, rt.createAuth(context.Background(), "1-alice", oldTokens))
	newTokens, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, "1-alice")
	require.NoError(t, err)

	err = rt.createAuth(context.Background(), "1-alice", newTokens)

	require.NoError(t, err)
	require.False(t, server.Exists(rt.wrapJwtKey(oldTokens.AccessUuid)))
	require.False(t, server.Exists(rt.wrapJwtKey(oldTokens.RefreshUuid)))
	require.True(t, server.Exists(rt.wrapJwtKey(newTokens.AccessUuid)))
	require.True(t, server.Exists(rt.wrapJwtKey(newTokens.RefreshUuid)))
	members, err := server.SMembers(rt.wrapJwtKey("alice"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{rt.wrapJwtKey(newTokens.AccessUuid), rt.wrapJwtKey(newTokens.RefreshUuid)}, members)
}

func TestRefreshPost_ExtendsIdentityTokenTTL(t *testing.T) {
	rt, server := sessionRouter(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice"}).Error)
	tokens, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, "1-alice")
	require.NoError(t, err)
	require.NoError(t, rt.createAuth(context.Background(), "1-alice", tokens))
	require.NoError(t, rt.saveIdToken(context.Background(), 1, "identity-token"))
	server.FastForward(30 * time.Minute)

	w := identityRequest(t, rt, rt.refreshPost, refreshForm{RefreshToken: tokens.RefreshToken}, "0", nil)

	var got map[string]string
	require.Empty(t, identityReply(t, w, &got))
	require.NotEmpty(t, got["access_token"])
	require.NotEmpty(t, got["refresh_token"])
	require.False(t, server.Exists(rt.wrapJwtKey(tokens.AccessUuid)))
	require.False(t, server.Exists(rt.wrapJwtKey(tokens.RefreshUuid)))
	require.Equal(t, time.Hour, server.TTL(rt.wrapIdTokenKey(1)))
	value, err := server.Get(rt.wrapIdTokenKey(1))
	require.NoError(t, err)
	require.Equal(t, "identity-token", value)
}

func TestCaptchaVerify_ConsumesChallengeOnBothSuccessAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  string
		message string
	}{
		{name: "correct", answer: "42"},
		{name: "incorrect", answer: "41", message: "incorrect verification code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, server := sessionRouter(t)
			original := captchaStore
			captchaStore = nil
			t.Cleanup(func() { captchaStore = original })
			store := rt.newCaptchaRedisStore()
			require.Same(t, store, rt.newCaptchaRedisStore())
			require.NoError(t, store.Set("challenge", "42"))
			require.Equal(t, 5*time.Minute, server.TTL("challenge"))
			require.Equal(t, "42", store.Get("challenge", false))

			w := identityRequest(t, rt, rt.captchaVerify, CaptchaReqBody{Id: "challenge", VerifyValue: tc.answer}, "0", nil)

			require.Equal(t, tc.message, identityReply(t, w, nil))
			require.False(t, server.Exists("challenge"))
		})
	}
}

func TestIfShowCaptcha_ReflectsConfiguration(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			rt := &Router{}
			rt.HTTP.ShowCaptcha.Enable = enabled

			w := identityRequest(t, rt, rt.ifShowCaptcha, nil, "0", nil)

			var got map[string]bool
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, map[string]bool{"show": enabled}, got)
		})
	}
}

func TestRSAConfigGet_EncodesPublicKey(t *testing.T) {
	rt := &Router{}
	rt.HTTP.RSA.OpenRSA = true
	rt.HTTP.RSA.RSAPublicKey = []byte("public-key")

	w := identityRequest(t, rt, rt.rsaConfigGet, nil, "0", nil)

	var response struct {
		Dat RSAConfigOutput `json:"dat"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Dat.OpenRSA)
	require.Equal(t, "cHVibGljLWtleQ==", response.Dat.RSAPublicKey)
}
