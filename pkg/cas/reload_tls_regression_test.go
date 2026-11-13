package cas

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Reloading secure TLS settings regressed to retaining the previous certificate-verification bypass.
func TestSsoClientReloadRestoresTLSVerificationRegression(t *testing.T) {
	client := New(Config{Enable: true})
	client.Reload(Config{Enable: true, SkipTlsVerify: true})
	require.NotNil(t, client.HTTPClient)
	transport, ok := client.HTTPClient.Transport.(*http.Transport)
	require.True(t, ok)
	t.Cleanup(transport.CloseIdleConnections)
	require.NotNil(t, transport.TLSClientConfig)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)

	client.Reload(Config{Enable: true, SkipTlsVerify: false})

	require.Nil(t, client.HTTPClient)
}
