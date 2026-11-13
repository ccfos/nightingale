package secu

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAES_RoundTripsDifferentKeyAndPlaintextLengths(t *testing.T) {
	for _, size := range []int{16, 24, 32} {
		key := make([]byte, size)
		_, err := rand.Read(key)
		require.NoError(t, err)
		for _, plain := range []string{"", "password", "0123456789abcdef", "监控密钥"} {
			encrypted, err := AesEncrypt([]byte(plain), key)

			require.NoError(t, err)
			require.NotEmpty(t, encrypted)
			require.Zero(t, len(encrypted)%16)
			decoded, err := AesDecrypt(encrypted, key)
			require.NoError(t, err)
			require.Equal(t, plain, string(decoded))
		}
	}
}

func TestPKCS7Padding_AddsFullBlockForAlignedInput(t *testing.T) {
	input := []byte("0123456789abcdef")

	padded := PKCS7Padding(input, 16)

	require.Equal(t, append([]byte("0123456789abcdef"), bytes.Repeat([]byte{16}, 16)...), padded)
	require.Equal(t, input, PKCS7UnPadding(padded))
}

func TestCipherConfig_RoundTripsMarkedValue(t *testing.T) {
	key := "0123456789abcdef"

	encrypted, err := DealWithEncrypt("secret 密码", key)

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(encrypted, "{{cipher}}"))
	plain, err := DealWithDecrypt(encrypted, key)
	require.NoError(t, err)
	require.Equal(t, "secret 密码", plain)
}

func TestDealWithDecrypt_PreservesPlainValue(t *testing.T) {
	plain, err := DealWithDecrypt("ordinary value", "unused")

	require.NoError(t, err)
	require.Equal(t, "ordinary value", plain)
}

func TestDealWithDecrypt_PreservesInputOnInvalidKeyOrEncoding(t *testing.T) {
	tests := []struct {
		name string
		src  string
		key  string
	}{
		{name: "invalid base64", src: "{{cipher}}%%%", key: "0123456789abcdef"},
		{name: "invalid key", src: "{{cipher}}YWJj", key: "short"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DealWithDecrypt(tt.src, tt.key)

			require.Error(t, err)
			require.Equal(t, tt.src, got)
		})
	}
}

func TestDealWithEncrypt_PreservesInputOnInvalidKey(t *testing.T) {
	got, err := DealWithEncrypt("secret", "short")

	require.Error(t, err)
	require.Equal(t, "secret", got)
}

func TestRSA_RoundTripsEncryptedAndUnencryptedPrivateKeys(t *testing.T) {
	for _, password := range []string{"", "key password"} {
		t.Run("password="+password, func(t *testing.T) {
			private, public, err := GenerateRsaKeyPair(password)
			require.NoError(t, err)
			block, _ := pem.Decode(private)
			require.NotNil(t, block)
			require.Equal(t, password != "", x509.IsEncryptedPEMBlock(block))

			encrypted, err := EncryptValue("database 密码", public)

			require.NoError(t, err)
			require.True(t, strings.HasPrefix(encrypted, "enc:"))
			for _, value := range []string{encrypted, strings.TrimPrefix(encrypted, "enc:")} {
				plain, err := Decrypt(value, private, password)
				require.NoError(t, err)
				require.Equal(t, "database 密码", plain)
			}
		})
	}
}

func TestDecrypt_RejectsInvalidPrivateKeys(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{name: "missing PEM", key: []byte("not PEM")},
		{name: "invalid DER", key: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("bad")})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain, err := Decrypt("YWJj", tt.key, "")

			require.Error(t, err)
			require.Empty(t, plain)
		})
	}
}

func TestDecrypt_RejectsWrongPassword(t *testing.T) {
	private, _, err := GenerateRsaKeyPair("password")
	require.NoError(t, err)

	_, err = Decrypt("YWJj", private, "wrong")

	require.Error(t, err)
}

func TestDecrypt_RejectsInvalidCiphertext(t *testing.T) {
	private, _, err := GenerateRsaKeyPair("password")
	require.NoError(t, err)

	_, err = Decrypt("YWJj", private, "password")

	require.Error(t, err)
}

func TestEncryptValue_RejectsInvalidDERAndNonRSAKeys(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	for _, value := range [][]byte{[]byte("invalid DER"), der} {
		public := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: value})

		encrypted, err := EncryptValue("secret", public)

		require.Error(t, err)
		require.Empty(t, encrypted)
	}
}

func TestEncryptValue_RejectsOversizedPlaintext(t *testing.T) {
	_, public, err := GenerateRsaKeyPair("")
	require.NoError(t, err)

	encrypted, err := EncryptValue(strings.Repeat("x", 256), public)

	require.ErrorContains(t, err, "failed to encrypt value")
	require.Empty(t, encrypted)
}
