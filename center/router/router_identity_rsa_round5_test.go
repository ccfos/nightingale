package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/stretchr/testify/require"
)

func identityRSAKeys(t *testing.T, rt *Router) []byte {
	t.Helper()
	private, public, err := secu.GenerateRsaKeyPair("test-password")
	require.NoError(t, err)
	rt.HTTP.RSA.OpenRSA = true
	rt.HTTP.RSA.RSAPrivateKey = private
	rt.HTTP.RSA.RSAPublicKey = public
	rt.HTTP.RSA.RSAPassWord = "test-password"
	return public
}

func TestUserAddPost_RSADecryptsPasswordBeforeHashing(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.Configs{})
	seedIdentitySalt(t, rt)
	public := identityRSAKeys(t, rt)
	cipher, err := secu.EncryptValue("secret", public)
	require.NoError(t, err)
	form := userAddForm{Username: "alice", Password: cipher, Roles: []string{"Viewer"}}

	w := identityRequest(t, rt, rt.userAddPost, form, "0", &models.User{Username: "operator"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	want, err := models.CryptoPass(rt.Ctx, "secret")
	require.NoError(t, err)
	require.Equal(t, want, saved.Password)
	require.NotEqual(t, cipher, saved.Password)
	require.Equal(t, "alice", saved.Username)
}

func TestUserAddPost_RSARejectsInvalidCiphertextWithoutCreatingUser(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.Configs{})
	identityRSAKeys(t, rt)
	form := userAddForm{Username: "alice", Password: "invalid", Roles: []string{"Viewer"}}

	w := identityRequest(t, rt, rt.userAddPost, form, "0", &models.User{Username: "operator"})

	require.Contains(t, identityReply(t, w, nil), "decryption error")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestUserPasswordPut_RSADecryptsSuppliedPassword(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.Configs{})
	seedIdentitySalt(t, rt)
	public := identityRSAKeys(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Password: "old", Roles: "Viewer"}).Error)
	cipher, err := secu.EncryptValue("new-password", public)
	require.NoError(t, err)

	w := identityRequest(t, rt, rt.userPasswordPut, userPasswordForm{Password: cipher}, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	want, err := models.CryptoPass(rt.Ctx, "new-password")
	require.NoError(t, err)
	require.Equal(t, want, saved.Password)
	require.Equal(t, "Viewer", saved.Roles)
	require.Equal(t, "operator", saved.UpdateBy)
}

func TestSelfPasswordPut_RSADecryptsBothPasswords(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.Configs{})
	seedIdentitySalt(t, rt)
	public := identityRSAKeys(t, rt)
	hash, err := models.CryptoPass(rt.Ctx, "old-password")
	require.NoError(t, err)
	user := models.User{Id: 1, Username: "alice", Password: hash}
	require.NoError(t, rt.Ctx.DB.Create(&user).Error)
	oldCipher, err := secu.EncryptValue("old-password", public)
	require.NoError(t, err)
	newCipher, err := secu.EncryptValue("new-password", public)
	require.NoError(t, err)

	w := identityRequest(t, rt, rt.selfPasswordPut, selfPasswordForm{OldPass: oldCipher, NewPass: newCipher}, "1", &user)

	require.Empty(t, identityReply(t, w, nil))
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	want, err := models.CryptoPass(rt.Ctx, "new-password")
	require.NoError(t, err)
	require.Equal(t, want, saved.Password)
	require.Equal(t, "alice", saved.UpdateBy)
}

func TestSelfPasswordPut_RSARejectsEitherInvalidPasswordWithoutMutation(t *testing.T) {
	for _, invalidOld := range []bool{false, true} {
		name := "new password"
		if invalidOld {
			name = "old password"
		}
		t.Run(name, func(t *testing.T) {
			rt := identityRouter(t, &models.User{}, &models.Configs{})
			public := identityRSAKeys(t, rt)
			user := models.User{Id: 1, Username: "alice", Password: "original"}
			require.NoError(t, rt.Ctx.DB.Create(&user).Error)
			validCipher, err := secu.EncryptValue("secret", public)
			require.NoError(t, err)
			form := selfPasswordForm{OldPass: validCipher, NewPass: "invalid"}
			if invalidOld {
				form = selfPasswordForm{OldPass: "invalid", NewPass: validCipher}
			}

			w := identityRequest(t, rt, rt.selfPasswordPut, form, "1", &user)

			require.Contains(t, identityReply(t, w, nil), "decryption error")
			var saved models.User
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, "original", saved.Password)
		})
	}
}

func TestLoginPost_RSAAuthenticatesDecryptedPassword(t *testing.T) {
	rt, server := sessionRouter(t)
	public := identityRSAKeys(t, rt)
	hash, err := models.CryptoPass(rt.Ctx, "secret")
	require.NoError(t, err)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Password: hash}).Error)
	cipher, err := secu.EncryptValue("secret", public)
	require.NoError(t, err)

	w := identityRequest(t, rt, rt.loginPost, loginForm{Username: "alice", Password: cipher}, "0", nil)

	var got struct {
		User         models.User `json:"user"`
		AccessToken  string      `json:"access_token"`
		RefreshToken string      `json:"refresh_token"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "alice", got.User.Username)
	require.NotEmpty(t, got.AccessToken)
	require.NotEmpty(t, got.RefreshToken)
	require.Len(t, server.Keys(), 2)
}

func TestLoginPost_RSARejectsInvalidCiphertextWithoutCreatingSession(t *testing.T) {
	rt, server := sessionRouter(t)
	identityRSAKeys(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Password: "original"}).Error)

	w := identityRequest(t, rt, rt.loginPost, loginForm{Username: "alice", Password: "invalid"}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "decryption error")
	require.Empty(t, server.Keys())
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Password)
}
