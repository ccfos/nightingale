package secu

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"

	"github.com/stretchr/testify/require"
)

// Malformed AES ciphertext regressed to panics or successful decryption with invalid padding.
func TestAesDecryptMalformedCiphertextRegression(t *testing.T) {
	key := []byte("0123456789abcdef")
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	badPadding := func(padding byte) []byte {
		plain := make([]byte, aes.BlockSize)
		plain[len(plain)-1] = padding
		encrypted := make([]byte, len(plain))
		cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(encrypted, plain)
		return encrypted
	}
	for _, tc := range []struct {
		name       string
		ciphertext []byte
	}{
		{name: "empty"},
		{name: "unaligned", ciphertext: make([]byte, aes.BlockSize+1)},
		{name: "zero padding", ciphertext: badPadding(0)},
		{name: "oversized padding", ciphertext: badPadding(aes.BlockSize + 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				_, err := AesDecrypt(tc.ciphertext, key)
				require.Error(t, err)
			})
		})
	}
	for _, plain := range []string{"", "secret", "0123456789abcdef"} {
		encrypted, err := AesEncrypt([]byte(plain), key)
		require.NoError(t, err)
		decrypted, err := AesDecrypt(encrypted, key)
		require.NoError(t, err)
		require.Equal(t, plain, string(decrypted))
	}
}
