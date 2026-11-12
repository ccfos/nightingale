package models

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/stretchr/testify/require"
)

func TestConfigsGet_PicksOldestBuiltinValueAndIgnoresExternalVariables(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	require.NoError(t, c.DB.Create(&[]Configs{
		{Id: 2, Ckey: "key", Cval: "newer"},
		{Id: 1, Ckey: "key", Cval: "oldest"},
		{Id: 3, Ckey: "key", Cval: "external", External: ConfigExternal},
		{Id: 4, Ckey: "", Cval: "blank"},
	}).Error)

	value, err := ConfigsGet(c, "key")

	require.NoError(t, err)
	require.Equal(t, "oldest", value)
	value, err = ConfigsGet(c, "missing")
	require.NoError(t, err)
	require.Empty(t, value)
	configs, err := ConfigsGetAll(c)
	require.NoError(t, err)
	require.ElementsMatch(t, []*Configs{
		{Id: 1, Ckey: "key", Cval: "oldest"},
		{Id: 2, Ckey: "key", Cval: "newer"},
	}, configs)
}

func TestConfigsSetWithUname_UpdatesValueAndPreservesCreationAudit(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	require.NoError(t, ConfigsSetWithUname(c, "key", "initial", "author"))

	require.NoError(t, ConfigsSetWithUname(c, "key", "updated", "editor"))

	configs, err := ConfigsSelectByCkey(c, "key")
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, "updated", configs[0].Cval)
	require.Equal(t, "author", configs[0].CreateBy)
	require.Equal(t, "editor", configs[0].UpdateBy)
	require.Zero(t, configs[0].External)
}

func TestConfigsSetWithUname_LeavesExternalVariableWithSameKey(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	require.NoError(t, c.DB.Create(&[]Configs{
		{Id: 1, Ckey: "shared", Cval: "builtin"},
		{Id: 2, Ckey: "shared", Cval: "external", External: ConfigExternal},
	}).Error)

	require.NoError(t, ConfigsSetWithUname(c, "shared", "updated", "editor"))

	var external Configs
	require.NoError(t, c.DB.First(&external, 2).Error)
	require.Equal(t, "external", external.Cval)
	var builtin Configs
	require.NoError(t, c.DB.First(&builtin, 1).Error)
	require.Equal(t, "updated", builtin.Cval)
}

func TestConfigsStatistics_SeparateBuiltinAndExternalWatermarks(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	require.NoError(t, c.DB.Create(&[]Configs{
		{Id: 1, Ckey: "builtin", UpdateAt: 100},
		{Id: 2, Ckey: "", UpdateAt: 900},
		{Id: 3, Ckey: "external", External: ConfigExternal, UpdateAt: 200},
	}).Error)

	builtin, err := ConfigCvalStatistics(c)
	require.NoError(t, err)
	external, err := ConfigsUserVariableStatistics(c)

	require.NoError(t, err)
	require.Equal(t, &Statistics{Total: 1, LastUpdated: 100}, builtin)
	require.Equal(t, &Statistics{Total: 1, LastUpdated: 200}, external)
}

func TestConfigUserVariableGetDecryptMap_DecryptsSecretsAndKeepsPlaintext(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	privateKey, publicKey, err := secu.GenerateRsaKeyPair("test-password")
	require.NoError(t, err)
	ciphertext, err := secu.EncryptValue("app-secret", publicKey)
	require.NoError(t, err)
	require.NoError(t, c.DB.Create(&[]Configs{
		{Ckey: "plain", Cval: "plain-value", External: ConfigExternal},
		{Ckey: "flashduty_app_key", Cval: ciphertext, External: ConfigExternal, Encrypted: ConfigEncrypted},
		{Ckey: "broken", Cval: "invalid ciphertext", External: ConfigExternal, Encrypted: ConfigEncrypted},
		{Ckey: "internal", Cval: "must not leak"},
		{Ckey: RSA_PRIVATE_KEY, Cval: string(privateKey)},
		{Ckey: RSA_PASSWORD, Cval: "test-password"},
	}).Error)

	values, err := ConfigUserVariableGetDecryptMap(c, privateKey, "test-password")

	require.NoError(t, err)
	require.Equal(t, map[string]string{"plain": "plain-value", "flashduty_app_key": "app-secret", "broken": ""}, values)
	appKey, err := ConfigsGetFlashDutyAppKey(c)
	require.NoError(t, err)
	require.Equal(t, "app-secret", appKey)
}

func TestConfigsGetSiteUrl_ValidatesStoredSiteInformation(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
		err   string
	}{
		{name: "empty", err: "site_info is empty"},
		{name: "malformed", value: "{", err: "failed to unmarshal site_info"},
		{name: "missing URL", value: `{}`, err: "site_url is empty in site_info"},
		{name: "wrong type", value: `{"site_url":123}`, err: "site_url is empty in site_info"},
		{name: "valid", value: `{"site_url":"https://monitor.example"}`, want: "https://monitor.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &Configs{})
			require.NoError(t, ConfigsSet(c, "site_info", tc.value))

			url, err := ConfigsGetSiteUrl(c)

			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, url)
			}
		})
	}
}

func TestPhoneEncryptionEnabled_ReadsLegacyValuesAndPersistsBoolean(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	for _, value := range []string{"true", "1", "false", "0", ""} {
		require.NoError(t, ConfigsSet(c, PHONE_ENCRYPTION_ENABLED, value))

		enabled, err := GetPhoneEncryptionEnabled(c)

		require.NoError(t, err)
		require.Equal(t, value == "true" || value == "1", enabled)
	}
	for _, enabled := range []bool{true, false} {
		require.NoError(t, SetPhoneEncryptionEnabled(c, enabled))
		stored, err := GetPhoneEncryptionEnabled(c)
		require.NoError(t, err)
		require.Equal(t, enabled, stored)
	}
}

func TestConfigsUserVariableInsert_RejectsInvalidAndDuplicateKeys(t *testing.T) {
	c := pipelineContext(t, &Configs{})
	for _, key := range []string{"bad-name", "1bad", "Host"} {
		err := ConfigsUserVariableInsert(c, Configs{Ckey: key})

		require.ErrorContains(t, err, "invalid key")
	}
	require.NoError(t, ConfigsUserVariableInsert(c, Configs{Ckey: "valid_key", Cval: "first"}))
	err := ConfigsUserVariableInsert(c, Configs{Ckey: "valid_key", Cval: "second"})
	require.EqualError(t, err, "duplicate ckey value found: valid_key")
	configs, err := ConfigsGetUserVariable(c)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, "first", configs[0].Cval)
}
