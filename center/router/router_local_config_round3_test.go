package router

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/center/router/agentassets"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConfPropCrypto_RoundTripsSupportedKeySizes(t *testing.T) {
	for _, key := range []string{"1234567890abcdef", "1234567890abcdefghijklmn", "1234567890abcdefghijklmnopqrstuv"} {
		t.Run(key, func(t *testing.T) {
			rt := &Router{}
			form := confPropCrypto{Data: "monitoring-secret", Key: key}

			encrypted := identityRequest(t, rt, rt.confPropEncrypt, form, "0", nil)

			require.Equal(t, http.StatusOK, encrypted.Code)
			var enc struct {
				Source string `json:"src"`
				Key    string `json:"key"`
				Cipher string `json:"encrypt"`
			}
			require.NoError(t, json.Unmarshal(encrypted.Body.Bytes(), &enc))
			require.Equal(t, form.Data, enc.Source)
			require.Equal(t, key, enc.Key)
			require.NotEmpty(t, enc.Cipher)
			require.NotEqual(t, form.Data, enc.Cipher)

			decrypted := identityRequest(t, rt, rt.confPropDecrypt, confPropCrypto{Data: enc.Cipher, Key: key}, "0", nil)

			require.Equal(t, http.StatusOK, decrypted.Code)
			var dec struct {
				Plain string `json:"decrypt"`
			}
			require.NoError(t, json.Unmarshal(decrypted.Body.Bytes(), &dec))
			require.Equal(t, form.Data, dec.Plain)
		})
	}
}

func TestConfPropCrypto_RejectsUnsupportedKeyLength(t *testing.T) {
	rt := &Router{}
	for _, handler := range []gin.HandlerFunc{rt.confPropEncrypt, rt.confPropDecrypt} {
		w := identityRequest(t, rt, handler, confPropCrypto{Data: "secret", Key: "short"}, "0", nil)

		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "The key length should be 16, 24 or 32", w.Body.String())
	}
}

func TestSmtpValidate_RequiresHostPortAndValidTOML(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		message string
	}{
		{name: "valid", text: "Host = 'localhost'\nPort = 2525\n"},
		{name: "missing host", text: "Port = 2525", message: "smtp host or port can not be empty"},
		{name: "missing port", text: "Host = 'localhost'", message: "smtp host or port can not be empty"},
		{name: "invalid syntax", text: "Host = [", message: "toml:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SmtpValidate(tc.text)

			if tc.message != "" {
				require.ErrorContains(t, err, tc.message)
			} else {
				require.NoError(t, err)
				require.Equal(t, "localhost", got.Host)
				require.Equal(t, 2525, got.Port)
			}
		})
	}
}

func TestNotifyConfigPut_RejectsUnsupportedKeysWithoutWriting(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	rt.NotifyConfigCache = &memsto.NotifyConfigCacheType{ConfigCache: &memsto.ConfigCache{}}
	rt.NotifyConfigCache.ConfigCache.Set(map[string]string{}, 0, 0)

	w := identityRequest(t, rt, rt.notifyConfigPut, models.Configs{Ckey: "forbidden", Cval: "value"}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "key forbidden can not modify")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Configs{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestNotifyConfigPut_RejectsMalformedSMTPBeforeWriting(t *testing.T) {
	rt := identityRouter(t, &models.Configs{})
	rt.NotifyConfigCache = &memsto.NotifyConfigCacheType{ConfigCache: &memsto.ConfigCache{}}
	rt.NotifyConfigCache.ConfigCache.Set(map[string]string{}, 0, 0)

	w := identityRequest(t, rt, rt.notifyConfigPut, models.Configs{Ckey: models.SMTP, Cval: "Host = ["}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "toml:")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Configs{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAttemptSendEmail_RejectsInvalidRecipientBeforeAccessingSender(t *testing.T) {
	rt := &Router{}

	w := identityRequest(t, rt, rt.attemptSendEmail, form{Email: "not-an-email"}, "0", nil)

	require.Contains(t, identityReply(t, w, nil), "email(not-an-email) invalid")
}

func TestCategrafCollectScript_ServesEmbeddedBytesAndDownloadHeaders(t *testing.T) {
	rt := &Router{}

	w := identityRequest(t, rt, rt.categrafCollectScript, nil, "0", nil)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, agentassets.CollectConfigScript, w.Body.String())
	require.Equal(t, "text/x-shellscript; charset=utf-8", w.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "no-store, no-cache, must-revalidate", w.Header().Get("Cache-Control"))
}

func TestCategrafDownload_ServesBundledArchive(t *testing.T) {
	rt := &Router{}
	dir, err := os.MkdirTemp("/tmp", "router-categraf-round3-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	rt.Center.AgentsDir = dir
	name := categrafPkgs["amd64"]
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("local archive fixture"), 0600))

	w := identityRequest(t, rt, rt.categrafDownload, nil, "0?arch=amd64", nil)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "local archive fixture", w.Body.String())
	require.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
	require.Contains(t, w.Header().Get("Content-Disposition"), name)
}

func TestCategrafDownload_ReportsUnsupportedOrMissingPackage(t *testing.T) {
	rt := &Router{}
	dir, err := os.MkdirTemp("/tmp", "router-categraf-missing-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	rt.Center.AgentsDir = dir

	unsupported := identityRequest(t, rt, rt.categrafDownload, nil, "0?arch=unknown", nil)
	missing := identityRequest(t, rt, rt.categrafDownload, nil, "0?arch=arm64", nil)

	require.Equal(t, http.StatusBadRequest, unsupported.Code)
	require.Contains(t, unsupported.Body.String(), "unsupported arch")
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), "not bundled")
}
