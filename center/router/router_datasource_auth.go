package router

import (
	"github.com/ccfos/nightingale/v6/pkg/dsauth"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/i18n"
)

type datasourceAuthPolicy struct {
	AllowAmbientCredentials bool     `json:"allow_ambient_credentials"`
	AllowAssumeRole         bool     `json:"allow_assume_role"`
	SigV4AllowedServices    []string `json:"sigv4_allowed_services"`
	// Default token scope per Azure cloud, used to prefill the form.
	AzureDefaultScopes map[string]string `json:"azure_default_scopes"`
}

// datasourceAuthPolicyGet tells the FE which cloud auth options this center
// accepts, so disabled ones can be greyed out instead of failing on save.
// Alert engines / edge read their own [DatasourceAuth] section and may differ.
func (rt *Router) datasourceAuthPolicyGet(c *gin.Context) {
	p := dsauth.GetPolicy()
	Render(c, datasourceAuthPolicy{
		AllowAmbientCredentials: !p.DisableAmbientCredentials,
		AllowAssumeRole:         !p.DisableAssumeRole,
		SigV4AllowedServices:    p.AllowedSigV4Services(),
		AzureDefaultScopes: map[string]string{
			dsauth.AzurePublic:     dsauth.AzureDefaultScope(dsauth.AzurePublic),
			dsauth.AzureChina:      dsauth.AzureDefaultScope(dsauth.AzureChina),
			dsauth.AzureGovernment: dsauth.AzureDefaultScope(dsauth.AzureGovernment),
		},
	}, nil)
}

// explainAuthError turns a dsauth error from the connectivity test into an
// actionable message. The machine-readable code is kept as a prefix so API
// callers (agents, scripts) can branch on it. Returns nil for other errors.
func explainAuthError(lang string, err error) error {
	code := dsauth.CodeOf(err)
	detail := sanitizeDsError(err.Error())
	switch code {
	case dsauth.CodeNoCredentials:
		return newMessageError("[" + code + "] " + i18n.Sprintf(lang,
			"no cloud credentials found in the n9e process: %s. Use an access key / client secret, or bind an IAM role / managed identity to the n9e pod or host", detail))
	case dsauth.CodeAssumeRoleFailed:
		return newMessageError("[" + code + "] " + i18n.Sprintf(lang,
			"failed to assume role: %s. Check the role trust policy and the external ID", detail))
	case dsauth.CodeTokenFailed:
		return newMessageError("[" + code + "] " + i18n.Sprintf(lang,
			"failed to obtain access token: %s. Check the client ID, client secret and token URL", detail))
	}
	return nil
}

func explainAccessDenied(lang string, status int, body string) error {
	return newMessageError("[" + dsauth.CodeAccessDenied + "] " + i18n.Sprintf(lang,
		"credentials were accepted but access is denied (HTTP %d), grant the identity query permission on the datasource, e.g. aps:QueryMetrics or Monitoring Data Reader: %s", status, sanitizeDsError(body)))
}
