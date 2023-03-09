package tlsx

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/choice"
)

// ClientConfig represents the standard client TLS config.
type ClientConfig struct {
	UseTLS             bool
	TLSCA              string
	TLSCert            string
	TLSKey             string
	TLSKeyPwd          string
	InsecureSkipVerify bool
	ServerName         string
	TLSMinVersion      string
	TLSMaxVersion      string
}

// ServerConfig represents the standard server TLS config.
type ServerConfig struct {
	TLSCert            string
	TLSKey             string
	TLSKeyPwd          string
	TLSAllowedCACerts  []string
	TLSCipherSuites    []string
	TLSMinVersion      string
	TLSMaxVersion      string
	TLSAllowedDNSNames []string
}

// TLSConfig returns a tls.Config, may be nil without error if TLS is not
// configured.
func (c *ClientConfig) TLSConfig() (*tls.Config, error) {
	if !c.UseTLS {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		Renegotiation:      tls.RenegotiateNever,
	}

	if c.TLSCA != "" {
		pool, err := makeCertPool([]string{c.TLSCA})
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = pool
	}

	if c.TLSCert != "" && c.TLSKey != "" {
		err := loadCertificate(tlsConfig, c.TLSCert, c.TLSKey)
		if err != nil {
			return nil, err
		}
	}

	if c.ServerName != "" {
		tlsConfig.ServerName = c.ServerName
	}

	if c.TLSMinVersion == "1.0" {
		tlsConfig.MinVersion = tls.VersionTLS10
	} else if c.TLSMinVersion == "1.1" {
		tlsConfig.MinVersion = tls.VersionTLS11
	} else if c.TLSMinVersion == "1.2" {
		tlsConfig.MinVersion = tls.VersionTLS12
	} else if c.TLSMinVersion == "1.3" {
		tlsConfig.MinVersion = tls.VersionTLS13
	}

	if c.TLSMaxVersion == "1.0" {
		tlsConfig.MaxVersion = tls.VersionTLS10
	} else if c.TLSMaxVersion == "1.1" {
		tlsConfig.MaxVersion = tls.VersionTLS11
	} else if c.TLSMaxVersion == "1.2" {
		tlsConfig.MaxVersion = tls.VersionTLS12
	} else if c.TLSMaxVersion == "1.3" {
		tlsConfig.MaxVersion = tls.VersionTLS13
	}

	return tlsConfig, nil
}

// TLSConfig returns a tls.Config, may be nil without error if TLS is not
// configured.
func (c *ServerConfig) TLSConfig() (*tls.Config, error) {
	if c.TLSCert == "" && c.TLSKey == "" && len(c.TLSAllowedCACerts) == 0 {
		return nil, nil
	}

	tlsConfig := &tls.Config{}

	if len(c.TLSAllowedCACerts) != 0 {
		pool, err := makeCertPool(c.TLSAllowedCACerts)
		if err != nil {
			return nil, err
		}
		tlsConfig.ClientCAs = pool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if c.TLSCert != "" && c.TLSKey != "" {
		err := loadCertificate(tlsConfig, c.TLSCert, c.TLSKey)
		if err != nil {
			return nil, err
		}
	}

	if len(c.TLSCipherSuites) != 0 {
		cipherSuites, err := ParseCiphers(c.TLSCipherSuites)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse server cipher suites %s: %v", strings.Join(c.TLSCipherSuites, ","), err)
		}
		tlsConfig.CipherSuites = cipherSuites
	}

	if c.TLSMaxVersion != "" {
		version, err := ParseTLSVersion(c.TLSMaxVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse tls max version %q: %v", c.TLSMaxVersion, err)
		}
		tlsConfig.MaxVersion = version
	}

	if c.TLSMinVersion != "" {
		version, err := ParseTLSVersion(c.TLSMinVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse tls min version %q: %v", c.TLSMinVersion, err)
		}
		tlsConfig.MinVersion = version
	}

	if tlsConfig.MinVersion != 0 && tlsConfig.MaxVersion != 0 && tlsConfig.MinVersion > tlsConfig.MaxVersion {
		return nil, fmt.Errorf(
			"tls min version %q can't be greater than tls max version %q", tlsConfig.MinVersion, tlsConfig.MaxVersion)
	}

	// Since clientAuth is tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	// there must be certs to validate.
	if len(c.TLSAllowedCACerts) > 0 && len(c.TLSAllowedDNSNames) > 0 {
		tlsConfig.VerifyPeerCertificate = c.verifyPeerCertificate
	}

	return tlsConfig, nil
}

func makeCertPool(certFiles []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, certFile := range certFiles {
		pem, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf(
				"could not read certificate %q: %v", certFile, err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf(
				"could not parse any PEM certificates %q: %v", certFile, err)
		}
	}
	return pool, nil
}

func loadCertificate(config *tls.Config, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf(
			"could not load keypair %s:%s: %v", certFile, keyFile, err)
	}

	config.Certificates = []tls.Certificate{cert}
	config.BuildNameToCertificate()
	return nil
}

func (c *ServerConfig) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// The certificate chain is client + intermediate + root.
	// Let's review the client certificate.
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("could not validate peer certificate: %v", err)
	}

	for _, name := range cert.DNSNames {
		if choice.Contains(name, c.TLSAllowedDNSNames) {
			return nil
		}
	}

	return fmt.Errorf("peer certificate not in allowed DNS Name list: %v", cert.DNSNames)
}
