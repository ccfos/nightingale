package secu

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Encrypting with non-PEM public key data regressed to a nil-pointer panic.
func TestEncryptValueNonPEMKeyRegression(t *testing.T) {
	require.NotPanics(t, func() {
		_, err := EncryptValue("secret", []byte("not a PEM public key"))
		require.Error(t, err)
	})
}
