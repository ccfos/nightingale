package dsauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"golang.org/x/oauth2"
)

type azureCloud struct {
	cfg cloud.Configuration
	// Default token scope for Azure Monitor managed Prometheus query API.
	scope string
}

var azureClouds = map[string]azureCloud{
	AzurePublic:     {cloud.AzurePublic, "https://prometheus.monitor.azure.com/.default"},
	AzureChina:      {cloud.AzureChina, "https://prometheus.monitor.azure.cn/.default"},
	AzureGovernment: {cloud.AzureGovernment, "https://prometheus.monitor.azure.us/.default"},
}

// AzureDefaultScope returns the default token scope of an Azure cloud, used by
// the FE to prefill the scope field.
func AzureDefaultScope(cloudName string) string {
	return azureClouds[cloudName].scope
}

func newAzureRoundTripper(base http.RoundTripper, c AzureConfig) (http.RoundTripper, error) {
	cred, err := newAzureCredential(c)
	if err != nil {
		return nil, err
	}
	scope := c.Scope
	if scope == "" {
		scope = azureClouds[c.Cloud].scope
	}
	return newTokenRoundTripper(base, &azureTokenSource{cred: cred, scope: scope}), nil
}

func newAzureCredential(c AzureConfig) (azcore.TokenCredential, error) {
	opts := azcore.ClientOptions{Cloud: azureClouds[c.Cloud].cfg}

	switch c.CredentialType {
	case AzureCredentialClientSecret:
		return azidentity.NewClientSecretCredential(c.TenantId, c.ClientId, c.ClientSecret,
			&azidentity.ClientSecretCredentialOptions{ClientOptions: opts})
	case AzureCredentialManagedIdentity:
		o := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: opts}
		if c.ClientId != "" {
			o.ID = azidentity.ClientID(c.ClientId)
		}
		return azidentity.NewManagedIdentityCredential(o)
	case AzureCredentialWorkloadIdentity:
		// Empty tenant / client id fall back to AZURE_TENANT_ID / AZURE_CLIENT_ID
		// injected by the AKS workload identity webhook.
		return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: opts,
			TenantID:      c.TenantId,
			ClientID:      c.ClientId,
		})
	}
	return nil, fmt.Errorf("unknown azure credential_type %q", c.CredentialType)
}

// azureTokenSource adapts an azcore credential to oauth2.TokenSource so the
// shared tokenRoundTripper can cache and refresh it.
type azureTokenSource struct {
	cred  azcore.TokenCredential
	scope string
}

func (s *azureTokenSource) Token() (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tokenRequestTimeout)
	defer cancel()
	tok, err := s.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{s.scope}})
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{AccessToken: tok.Token, TokenType: "Bearer", Expiry: tok.ExpiresOn}, nil
}
