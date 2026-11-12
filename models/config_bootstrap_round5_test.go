package models

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/stretchr/testify/require"
)

func TestInitJWTSigningKey_PersistsAndReusesGeneratedKey(t *testing.T) {
	c := pipelineContext(t, &Configs{})

	first := InitJWTSigningKey(c)
	second := InitJWTSigningKey(c)

	require.Len(t, first, 32)
	require.Equal(t, first, second)
	stored, err := ConfigsGet(c, JWT_SIGNING_KEY)
	require.NoError(t, err)
	require.Equal(t, first, stored)
	rows, err := ConfigsSelectByCkey(c, JWT_SIGNING_KEY)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestInitSalt_PreservesGeneratedSaltAcrossRestarts(t *testing.T) {
	c := pipelineContext(t, &Configs{})

	InitSalt(c)
	first, err := ConfigsGet(c, SALT)
	require.NoError(t, err)
	InitSalt(c)
	second, err := ConfigsGet(c, SALT)

	require.NoError(t, err)
	require.Len(t, first, 32)
	require.Equal(t, first, second)
	rows, err := ConfigsSelectByCkey(c, SALT)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestInitRSAPassWord_PersistsAndReusesGeneratedPassword(t *testing.T) {
	c := pipelineContext(t, &Configs{})

	first, err := InitRSAPassWord(c)
	require.NoError(t, err)
	second, err := InitRSAPassWord(c)

	require.NoError(t, err)
	require.Len(t, first, 32)
	require.Equal(t, first, second)
	stored, err := ConfigsGet(c, RSA_PASSWORD)
	require.NoError(t, err)
	require.Equal(t, first, stored)
}

func TestInitRSAPassWord_ReportsConfigurationReadFailure(t *testing.T) {
	c := pipelineContext(t, &Role{})

	password, err := InitRSAPassWord(c)

	require.ErrorContains(t, err, "failed to get rsa password")
	require.Empty(t, password)
}

func TestConfigsAdd_SeparatesBuiltinAndExternalNamespaces(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	builtin := Configs{Ckey: "shared", Cval: "builtin", CreateAt: 10, CreateBy: "author"}
	external := Configs{Ckey: "shared", Cval: "external", External: 1, CreateAt: 20, CreateBy: "editor"}

	require.NoError(t, builtin.Add(c))
	require.NoError(t, external.Add(c))
	err := (&Configs{Ckey: "shared", Cval: "duplicate"}).Add(c)

	require.ErrorContains(t, err, "key is exists")
	rows, err := ConfigsSelectByCkey(c, "shared")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "builtin", rows[0].Cval)
	require.Equal(t, "author", rows[0].UpdateBy)
	require.Equal(t, int64(10), rows[0].UpdateAt)
	require.Equal(t, "external", rows[1].Cval)
	require.Equal(t, 1, rows[1].External)
}

func TestConfigsUpdate_RejectsConflictsWithinNamespace(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	require.NoError(t, c.DB.Create(&[]Configs{
		{Id: 1, Ckey: "first", Cval: "old"},
		{Id: 2, Ckey: "second", Cval: "retained"},
		{Id: 3, Ckey: "external", External: 1},
	}).Error)
	update := Configs{Id: 1, Ckey: "second", Cval: "discarded"}

	err := update.Update(c)

	require.ErrorContains(t, err, "key is exists")
	row, err := ConfigGet(c, 1)
	require.NoError(t, err)
	require.Equal(t, "first", row.Ckey)
	require.Equal(t, "old", row.Cval)

	update.Ckey = "external"
	update.Cval = "new"
	require.NoError(t, update.Update(c))

	row, err = ConfigGet(c, 1)
	require.NoError(t, err)
	require.Equal(t, "external", row.Ckey)
	require.Equal(t, "new", row.Cval)
}

func TestConfigsGets_AppliesPrefixBeforePagination(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	require.NoError(t, c.DB.Create(&[]Configs{
		{Id: 1, Ckey: "app_one"},
		{Id: 2, Ckey: "unrelated"},
		{Id: 3, Ckey: "app_two"},
		{Id: 4, Ckey: "app_three"},
	}).Error)

	rows, err := ConfigsGets(c, "app_", 1, 1)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(3), rows[0].Id)
	all, err := ConfigsGets(c, "", 10, 0)
	require.NoError(t, err)
	require.Len(t, all, 4)
	require.Equal(t, int64(4), all[0].Id)
}

func preservePhoneEncryptionCache(t *testing.T) {
	t.Helper()
	enabled, publicKey, privateKey, password, loaded := GetPhoneEncryptionConfigFromCache()
	t.Cleanup(func() {
		phoneEncryptionCache.Lock()
		defer phoneEncryptionCache.Unlock()
		phoneEncryptionCache.enabled = enabled
		phoneEncryptionCache.publicKey = publicKey
		phoneEncryptionCache.privateKey = privateKey
		phoneEncryptionCache.password = password
		phoneEncryptionCache.loaded = loaded
	})
}

func TestPhoneEncryptionCache_LoadsAndRefreshesStoredConfiguration(t *testing.T) {
	preservePhoneEncryptionCache(t)
	c := pipelineContext(t, &Configs{})
	require.NoError(t, ConfigsSet(c, PHONE_ENCRYPTION_ENABLED, "true"))
	require.NoError(t, ConfigsSet(c, RSA_PRIVATE_KEY, "private-key"))
	require.NoError(t, ConfigsSet(c, RSA_PUBLIC_KEY, "public-key"))
	require.NoError(t, ConfigsSet(c, RSA_PASSWORD, "password"))

	require.NoError(t, LoadPhoneEncryptionConfig(c))

	enabled, public, private, password, loaded := GetPhoneEncryptionConfigFromCache()
	require.True(t, enabled)
	require.True(t, loaded)
	require.Equal(t, []byte("private-key"), private)
	require.Equal(t, []byte("public-key"), public)
	require.Equal(t, "password", password)
	require.NoError(t, ConfigsSet(c, PHONE_ENCRYPTION_ENABLED, "false"))
	require.NoError(t, ConfigsSet(c, RSA_PUBLIC_KEY, "rotated-key"))

	require.NoError(t, RefreshPhoneEncryptionCache(c))

	enabled, public, _, _, loaded = GetPhoneEncryptionConfigFromCache()
	require.False(t, enabled)
	require.True(t, loaded)
	require.Equal(t, []byte("rotated-key"), public)
}

func TestUserPhoneEncryption_RoundTripsAndDoesNotDoubleEncrypt(t *testing.T) {
	preservePhoneEncryptionCache(t)
	private, public, err := secu.GenerateRsaKeyPair("round5-password")
	require.NoError(t, err)
	phoneEncryptionCache.Lock()
	phoneEncryptionCache.enabled = true
	phoneEncryptionCache.loaded = true
	phoneEncryptionCache.privateKey = private
	phoneEncryptionCache.publicKey = public
	phoneEncryptionCache.password = "round5-password"
	phoneEncryptionCache.Unlock()
	user := User{Username: "alice", Phone: "123456789"}

	require.NoError(t, user.EncryptPhone())

	require.Contains(t, user.Phone, "enc:")
	require.NotEqual(t, "123456789", user.Phone)
	encrypted := user.Phone
	require.NoError(t, user.EncryptPhone())
	require.Equal(t, encrypted, user.Phone)
	user.DecryptPhone()
	require.Equal(t, "123456789", user.Phone)
}

func TestUserEncryptPhone_DisabledEncryptionPreservesPlaintext(t *testing.T) {
	preservePhoneEncryptionCache(t)
	phoneEncryptionCache.Lock()
	phoneEncryptionCache.loaded = true
	phoneEncryptionCache.enabled = false
	phoneEncryptionCache.Unlock()
	user := User{Phone: "123456789"}

	require.NoError(t, user.EncryptPhone())

	require.Equal(t, "123456789", user.Phone)
}

func TestUserDecryptPhone_DisabledEncryptionPreservesCiphertext(t *testing.T) {
	preservePhoneEncryptionCache(t)
	phoneEncryptionCache.Lock()
	phoneEncryptionCache.loaded = true
	phoneEncryptionCache.enabled = false
	phoneEncryptionCache.Unlock()
	user := User{Phone: "enc:opaque"}

	user.DecryptPhone()

	require.Equal(t, "enc:opaque", user.Phone)
}
