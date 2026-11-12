package tlsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func tlsFiles(t *testing.T) (string, string, []byte) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "n9e-tlsx-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "agent.local"},
		DNSNames: []string{"agent.local", "other.local"}, IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0),
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile, der
}

func TestTLSConfig_DisabledReturnsNil(t *testing.T) {
	client, err := (&ClientConfig{TLSCA: "unused"}).TLSConfig()

	require.NoError(t, err)
	require.Nil(t, client)
	server, err := (&ServerConfig{}).TLSConfig()
	require.NoError(t, err)
	require.Nil(t, server)
}

func TestClientTLSConfig_LoadsTrustAndIdentity(t *testing.T) {
	cert, key, der := tlsFiles(t)
	c := &ClientConfig{
		UseTLS: true, TLSCA: cert, TLSCert: cert, TLSKey: key, ServerName: "agent.local",
		InsecureSkipVerify: true, TLSMinVersion: "1.2", TLSMaxVersion: "1.3",
	}

	config, err := c.TLSConfig()

	require.NoError(t, err)
	require.True(t, config.InsecureSkipVerify)
	require.Equal(t, tls.RenegotiateNever, config.Renegotiation)
	require.Equal(t, "agent.local", config.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
	require.Equal(t, uint16(tls.VersionTLS13), config.MaxVersion)
	require.Len(t, config.Certificates, 1)
	require.Equal(t, der, config.Certificates[0].Certificate[0])
	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	_, err = parsed.Verify(x509.VerifyOptions{Roots: config.RootCAs, CurrentTime: time.Unix(1700000000, 0)})
	require.NoError(t, err)
}

func TestClientTLSConfig_MapsVersionBounds(t *testing.T) {
	tests := []struct {
		version string
		want    uint16
	}{
		{version: "1.0", want: tls.VersionTLS10},
		{version: "1.1", want: tls.VersionTLS11},
		{version: "1.2", want: tls.VersionTLS12},
		{version: "1.3", want: tls.VersionTLS13},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			c := &ClientConfig{UseTLS: true, TLSMinVersion: tt.version, TLSMaxVersion: tt.version}

			config, err := c.TLSConfig()

			require.NoError(t, err)
			require.Equal(t, tt.want, config.MinVersion)
			require.Equal(t, tt.want, config.MaxVersion)
		})
	}
}

func TestServerTLSConfig_RequiresTrustedClientWithAllowedDNS(t *testing.T) {
	cert, key, der := tlsFiles(t)
	c := &ServerConfig{
		TLSCert: cert, TLSKey: key, TLSAllowedCACerts: []string{cert}, TLSAllowedDNSNames: []string{"agent.local"},
		TLSMinVersion: "TLS12", TLSMaxVersion: "TLS13",
		TLSCipherSuites: []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"},
	}

	config, err := c.TLSConfig()

	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, config.ClientAuth)
	require.NotNil(t, config.ClientCAs)
	require.Len(t, config.Certificates, 1)
	require.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
	require.Equal(t, uint16(tls.VersionTLS13), config.MaxVersion)
	require.Equal(t, []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}, config.CipherSuites)
	require.NotNil(t, config.VerifyPeerCertificate)
	require.NoError(t, config.VerifyPeerCertificate([][]byte{der}, nil))
}

func TestVerifyPeerCertificate_RejectsUnlistedDNSName(t *testing.T) {
	_, _, der := tlsFiles(t)
	c := &ServerConfig{TLSAllowedDNSNames: []string{"unrelated.local"}}

	err := c.verifyPeerCertificate([][]byte{der}, nil)

	require.ErrorContains(t, err, "not in allowed DNS")
}

func TestVerifyPeerCertificate_RejectsMalformedDER(t *testing.T) {
	c := &ServerConfig{TLSAllowedDNSNames: []string{"agent.local"}}

	err := c.verifyPeerCertificate([][]byte{[]byte("bad DER")}, nil)

	require.ErrorContains(t, err, "could not validate")
}

func TestServerTLSConfig_RejectsUnsupportedOrReversedBounds(t *testing.T) {
	cert, _, _ := tlsFiles(t)
	tests := []struct {
		name string
		min  string
		max  string
	}{
		{name: "minimum", min: "TLS99"},
		{name: "maximum", max: "TLS99"},
		{name: "reversed", min: "TLS13", max: "TLS12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ServerConfig{TLSAllowedCACerts: []string{cert}, TLSMinVersion: tt.min, TLSMaxVersion: tt.max}

			config, err := c.TLSConfig()

			require.Error(t, err)
			require.Nil(t, config)
		})
	}
}

func TestServerTLSConfig_RejectsUnknownCipher(t *testing.T) {
	cert, _, _ := tlsFiles(t)
	c := &ServerConfig{TLSAllowedCACerts: []string{cert}, TLSCipherSuites: []string{"unknown"}}

	config, err := c.TLSConfig()

	require.ErrorContains(t, err, "could not parse server cipher suites")
	require.Nil(t, config)
}

func TestTLSConfig_RejectsMissingCAFile(t *testing.T) {
	cert, _, _ := tlsFiles(t)
	missing := cert + ".missing"

	_, err := (&ClientConfig{UseTLS: true, TLSCA: missing}).TLSConfig()

	require.ErrorContains(t, err, "could not read certificate")
	_, err = (&ServerConfig{TLSAllowedCACerts: []string{missing}}).TLSConfig()
	require.ErrorContains(t, err, "could not read certificate")
}

func TestTLSConfig_RejectsMalformedCAFile(t *testing.T) {
	_, key, _ := tlsFiles(t)

	_, err := (&ClientConfig{UseTLS: true, TLSCA: key}).TLSConfig()

	require.ErrorContains(t, err, "could not parse any PEM certificates")
	_, err = (&ServerConfig{TLSAllowedCACerts: []string{key}}).TLSConfig()
	require.ErrorContains(t, err, "could not parse any PEM certificates")
}

func TestTLSConfig_RejectsInvalidKeyPair(t *testing.T) {
	cert, _, _ := tlsFiles(t)

	_, err := (&ClientConfig{UseTLS: true, TLSCert: cert, TLSKey: cert}).TLSConfig()

	require.ErrorContains(t, err, "could not load keypair")
	_, err = (&ServerConfig{TLSCert: cert, TLSKey: cert}).TLSConfig()
	require.ErrorContains(t, err, "could not load keypair")
}

func TestParseCiphers_PreservesOrder(t *testing.T) {
	got, err := ParseCiphers([]string{"TLS_AES_256_GCM_SHA384", "TLS_AES_128_GCM_SHA256"})

	require.NoError(t, err)
	require.Equal(t, []uint16{tls.TLS_AES_256_GCM_SHA384, tls.TLS_AES_128_GCM_SHA256}, got)
}

func TestParseCiphers_RejectsUnknownNames(t *testing.T) {
	got, err := ParseCiphers([]string{"TLS_AES_256_GCM_SHA384", "unknown"})

	require.ErrorContains(t, err, "unsupported cipher")
	require.Nil(t, got)
}
