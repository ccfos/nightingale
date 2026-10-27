// Package dsauth adds pluggable request authentication (AWS SigV4, OAuth2
// client credentials, Azure Entra ID) to datasource HTTP clients.
//
// It is a leaf package on purpose: models, dskit and the datasource plugins
// all import it, so it must not import any of them.
package dsauth

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
)

const (
	TypeNone   = ""
	TypeBasic  = "basic"
	TypeSigV4  = "sigv4"
	TypeOAuth2 = "oauth2"
	TypeAzure  = "azure"
)

const (
	AWSCredentialDefault = "default" // SDK default chain: env, IRSA, Pod Identity, ECS task role, EC2 instance profile
	AWSCredentialKeys    = "keys"    // static access key / secret key

	AzureCredentialClientSecret     = "client_secret"
	AzureCredentialManagedIdentity  = "managed_identity"
	AzureCredentialWorkloadIdentity = "workload_identity"

	AzurePublic     = "AzurePublic"
	AzureChina      = "AzureChina"
	AzureGovernment = "AzureGovernment"
)

// Config is the cloud part of a datasource's auth settings. Field names follow
// Prometheus' http_config (sigv4 / oauth2 / azuread) so users can map them 1:1.
type Config struct {
	Type   string        `json:"type,omitempty"`
	SigV4  *SigV4Config  `json:"sigv4,omitempty"`
	OAuth2 *OAuth2Config `json:"oauth2,omitempty"`
	Azure  *AzureConfig  `json:"azure,omitempty"`
}

type SigV4Config struct {
	Region         string `json:"region"`
	Service        string `json:"service_name"`    // default "aps"
	CredentialType string `json:"credential_type"` // "default" | "keys"
	AccessKey      string `json:"access_key,omitempty"`
	SecretKey      string `json:"secret_key,omitempty"`
	RoleArn        string `json:"role_arn,omitempty"`
	ExternalId     string `json:"external_id,omitempty"`
	StsEndpoint    string `json:"sts_endpoint,omitempty"` // optional: VPC endpoint / FIPS
}

type OAuth2Config struct {
	ClientId       string            `json:"client_id"`
	ClientSecret   string            `json:"client_secret"`
	TokenURL       string            `json:"token_url"`
	Scopes         []string          `json:"scopes,omitempty"`
	EndpointParams map[string]string `json:"endpoint_params,omitempty"`
}

type AzureConfig struct {
	Cloud          string `json:"cloud"`           // AzurePublic | AzureChina | AzureGovernment
	CredentialType string `json:"credential_type"` // client_secret | managed_identity | workload_identity
	TenantId       string `json:"tenant_id,omitempty"`
	ClientId       string `json:"client_id,omitempty"` // managed identity: user-assigned client id, empty = system-assigned
	ClientSecret   string `json:"client_secret,omitempty"`
	Scope          string `json:"scope,omitempty"` // empty = default by cloud
}

// IsCloud reports whether the config replaces Basic Auth with a token/signature
// based scheme. Callers use it to drop Basic credentials and reject a custom
// Authorization header.
func (c Config) IsCloud() bool {
	switch c.Type {
	case TypeSigV4, TypeOAuth2, TypeAzure:
		return true
	}
	return false
}

func (c Config) Equal(o Config) bool {
	return reflect.DeepEqual(c.normalized(), o.normalized())
}

// normalized drops the sub-configs that do not belong to Type, so leftovers
// from the FE form never affect equality or validation.
func (c Config) normalized() Config {
	n := Config{Type: c.Type}
	switch c.Type {
	case TypeSigV4:
		n.SigV4 = c.SigV4
	case TypeOAuth2:
		n.OAuth2 = c.OAuth2
	case TypeAzure:
		n.Azure = c.Azure
	}
	return n
}

// Normalize returns the config with defaults filled in and unrelated
// sub-configs removed. It is what gets persisted.
func (c Config) Normalize() Config {
	n := c.normalized()
	// "basic" is what the FE sends for the Basic Auth tab; store it as the
	// legacy empty type so rows compare equal to ones written before cloud
	// auth existed (e.g. InitEmbeddedTSDBDatasource's Auth comparison).
	if n.Type == TypeBasic {
		n.Type = TypeNone
	}
	switch n.Type {
	case TypeSigV4:
		if n.SigV4 == nil {
			return n
		}
		s := *n.SigV4
		s.Region = strings.TrimSpace(s.Region)
		s.Service = strings.TrimSpace(s.Service)
		if s.Service == "" {
			s.Service = "aps"
		}
		if s.CredentialType == "" {
			s.CredentialType = AWSCredentialDefault
		}
		if s.CredentialType != AWSCredentialKeys {
			s.AccessKey, s.SecretKey = "", ""
		}
		s.RoleArn = strings.TrimSpace(s.RoleArn)
		if s.RoleArn == "" {
			s.ExternalId = ""
		}
		n.SigV4 = &s
	case TypeOAuth2:
		if n.OAuth2 == nil {
			return n
		}
		o := *n.OAuth2
		o.TokenURL = strings.TrimSpace(o.TokenURL)
		n.OAuth2 = &o
	case TypeAzure:
		if n.Azure == nil {
			return n
		}
		a := *n.Azure
		if a.Cloud == "" {
			a.Cloud = AzurePublic
		}
		if a.CredentialType == "" {
			a.CredentialType = AzureCredentialClientSecret
		}
		if a.CredentialType != AzureCredentialClientSecret {
			a.ClientSecret = ""
		}
		n.Azure = &a
	}
	return n
}

// Policy is the process-level [DatasourceAuth] config section. Every process
// that queries datasources (center, alert, edge) reads its own copy.
type Policy struct {
	// Forbid using the identity of the host the process runs on: AWS default
	// credential chain, Azure Managed Identity / Workload Identity.
	DisableAmbientCredentials bool
	// Forbid AWS AssumeRole.
	DisableAssumeRole bool
	// SigV4 service names a datasource may sign for. Empty means DefaultSigV4Services.
	// Keeps the host identity from being used against arbitrary AWS APIs through
	// the datasource proxy.
	SigV4AllowedServices []string
}

var DefaultSigV4Services = []string{"aps", "es", "aoss"}

var currentPolicy atomic.Pointer[Policy]

func SetPolicy(p Policy) {
	currentPolicy.Store(&p)
}

func GetPolicy() Policy {
	if p := currentPolicy.Load(); p != nil {
		return *p
	}
	return Policy{}
}

func (p Policy) AllowedSigV4Services() []string {
	if len(p.SigV4AllowedServices) == 0 {
		return DefaultSigV4Services
	}
	return p.SigV4AllowedServices
}

// Validate checks a config against required fields and the current policy.
func Validate(c Config) error {
	return validate(c.Normalize(), GetPolicy())
}

func validate(c Config, p Policy) error {
	switch c.Type {
	case TypeNone, TypeBasic:
		return nil
	case TypeSigV4:
		s := c.SigV4
		if s == nil {
			return fmt.Errorf("sigv4 config is required")
		}
		if s.Region == "" {
			return fmt.Errorf("sigv4 region is required")
		}
		if !contains(p.AllowedSigV4Services(), s.Service) {
			return fmt.Errorf("sigv4 service %q is not allowed, allowed: %s", s.Service, strings.Join(p.AllowedSigV4Services(), ","))
		}
		switch s.CredentialType {
		case AWSCredentialKeys:
			if s.AccessKey == "" || s.SecretKey == "" {
				return fmt.Errorf("sigv4 access key and secret key are required")
			}
		case AWSCredentialDefault:
			if p.DisableAmbientCredentials {
				return fmt.Errorf("sigv4 default credential chain is disabled by DatasourceAuth.DisableAmbientCredentials")
			}
		default:
			return fmt.Errorf("unknown sigv4 credential_type %q", s.CredentialType)
		}
		if s.RoleArn != "" {
			if p.DisableAssumeRole {
				return fmt.Errorf("sigv4 assume role is disabled by DatasourceAuth.DisableAssumeRole")
			}
			if !strings.HasPrefix(s.RoleArn, "arn:") {
				return fmt.Errorf("invalid role arn %q", s.RoleArn)
			}
		}
		if s.StsEndpoint != "" {
			if err := checkHTTPURL(s.StsEndpoint); err != nil {
				return fmt.Errorf("invalid sts endpoint: %v", err)
			}
		}
	case TypeOAuth2:
		o := c.OAuth2
		if o == nil {
			return fmt.Errorf("oauth2 config is required")
		}
		if o.ClientId == "" || o.ClientSecret == "" {
			return fmt.Errorf("oauth2 client_id and client_secret are required")
		}
		if err := checkHTTPURL(o.TokenURL); err != nil {
			return fmt.Errorf("invalid oauth2 token_url: %v", err)
		}
	case TypeAzure:
		a := c.Azure
		if a == nil {
			return fmt.Errorf("azure config is required")
		}
		if _, ok := azureClouds[a.Cloud]; !ok {
			return fmt.Errorf("unknown azure cloud %q", a.Cloud)
		}
		switch a.CredentialType {
		case AzureCredentialClientSecret:
			if a.TenantId == "" || a.ClientId == "" || a.ClientSecret == "" {
				return fmt.Errorf("azure tenant_id, client_id and client_secret are required")
			}
		case AzureCredentialManagedIdentity, AzureCredentialWorkloadIdentity:
			if p.DisableAmbientCredentials {
				return fmt.Errorf("azure %s is disabled by DatasourceAuth.DisableAmbientCredentials", a.CredentialType)
			}
		default:
			return fmt.Errorf("unknown azure credential_type %q", a.CredentialType)
		}
	default:
		return fmt.Errorf("unknown auth type %q", c.Type)
	}
	return nil
}

// Wrap returns base wrapped with the authentication described by c. Types that
// do not need a RoundTripper (none / basic) return base unchanged. The policy
// is re-checked here so that tightening it also stops existing datasources.
func Wrap(base http.RoundTripper, c Config) (http.RoundTripper, error) {
	c = c.Normalize()
	if !c.IsCloud() {
		return base, nil
	}
	if err := validate(c, GetPolicy()); err != nil {
		return nil, err
	}
	if base == nil {
		base = http.DefaultTransport
	}

	switch c.Type {
	case TypeSigV4:
		return newSigV4RoundTripper(base, *c.SigV4)
	case TypeOAuth2:
		return newOAuth2RoundTripper(base, *c.OAuth2), nil
	case TypeAzure:
		return newAzureRoundTripper(base, *c.Azure)
	}
	return base, nil
}

func contains(lst []string, s string) bool {
	for _, v := range lst {
		if v == s {
			return true
		}
	}
	return false
}

func checkHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q must be an http(s) url", raw)
	}
	return nil
}

func closeIdle(rt http.RoundTripper) {
	if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
