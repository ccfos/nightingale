package a2a

import (
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	skillpkg "github.com/ccfos/nightingale/v6/aiagent/skill"
)

// AgentCardOptions configures the static AgentCard exposed at /.well-known/agent.json.
type AgentCardOptions struct {
	// BaseURL is the absolute URL prefix advertised in SupportedInterfaces.
	// When empty, the URL is derived per-request from Host + X-Forwarded-Proto.
	BaseURL string
	// A2APath is the URL path at which the A2A endpoint is mounted (e.g. "/a2a").
	A2APath string
	// TokenHeaderName is the header used for X-User-Token authentication, surfaced
	// to clients via the SecuritySchemes block.
	TokenHeaderName string
	// OIDCDiscoveryURL, when non-nil, is called per request to resolve the
	// trusted IdP's OpenID Connect discovery URL; a non-empty result makes the
	// card advertise an additional OpenID Connect security scheme so A2A clients
	// can discover the agent also accepts an OAuth access token from this IdP
	// (Resource Server mode). It is evaluated per request — not captured once —
	// so the card reflects live RS/OIDC config without a center restart.
	// Returning "" (or leaving this nil) advertises only X-User-Token.
	OIDCDiscoveryURL func() string
}

const (
	securitySchemeName     a2a.SecuritySchemeName = "x-user-token"
	oidcSecuritySchemeName a2a.SecuritySchemeName = "oidc"
)

// AgentCardHandler returns an http.Handler that serves the n9e AgentCard. The
// card is rebuilt per-request so BaseURL can be auto-derived when not configured.
func AgentCardHandler(opts AgentCardOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := opts.BaseURL
		if base == "" {
			scheme := "http"
			if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
				scheme = "https"
			}
			base = scheme + "://" + r.Host
		}
		base = strings.TrimSuffix(base, "/")
		path := opts.A2APath
		if path == "" {
			path = "/a2a"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		oidcDiscoveryURL := ""
		if opts.OIDCDiscoveryURL != nil {
			oidcDiscoveryURL = opts.OIDCDiscoveryURL()
		}
		card := buildAgentCard(base+path, opts.TokenHeaderName, oidcDiscoveryURL)
		a2asrv.NewStaticAgentCardHandler(card).ServeHTTP(w, r)
	})
}

func buildAgentCard(endpointURL, tokenHeader, oidcDiscoveryURL string) *a2a.AgentCard {
	if tokenHeader == "" {
		tokenHeader = "X-User-Token"
	}

	desc := "Operate the Nightingale observability platform via natural language: " +
		"query alert events/rules, dashboards, datasources, hosts, business groups, " +
		"and run AI-powered troubleshooting and inspection skills."

	skills := loadBuiltinAgentSkills()
	if len(skills) == 0 {
		// 内置 skill embed 读不出来时给一条聚合兜底，至少别返回空 Skills 数组
		// 让发现端 schema 校验过不去。
		skills = []a2a.AgentSkill{{
			ID:          "n9e-assistant",
			Name:        "Nightingale Assistant",
			Description: desc,
			Tags:        []string{"observability", "alerting", "monitoring"},
			// AgentCard 是面向上游 agent 的公开发现端点，无请求语言上下文，
			// 示例问句用英文（A2A 对话本身任意语言均可）。
			Examples: []string{
				"Show the currently firing alert events in the prod business group",
				"Run a root-cause analysis for alert #12345",
				"Summarize metric anomalies on host-01 over the past hour",
				"Create an alert rule for CPU > 80%",
			},
		}}
	}

	schemes := a2a.NamedSecuritySchemes{
		securitySchemeName: a2a.APIKeySecurityScheme{
			Description: "n9e user API token. Manage tokens under user profile in the n9e UI.",
			Location:    a2a.APIKeySecuritySchemeLocationHeader,
			Name:        tokenHeader,
		},
	}
	requirements := a2a.SecurityRequirementsOptions{
		a2a.SecurityRequirements{securitySchemeName: {}},
	}
	// When the OAuth Resource Server path is enabled, advertise an additional
	// OpenID Connect scheme so A2A clients can discover they may instead present
	// an IdP-issued OAuth access token (satisfy-any with x-user-token).
	if oidcDiscoveryURL != "" {
		schemes[oidcSecuritySchemeName] = a2a.OpenIDConnectSecurityScheme{
			Description:      "OAuth 2.0 access token from the configured enterprise IdP, sent as 'Authorization: Bearer'. Its audience must be bound to this service.",
			OpenIDConnectURL: oidcDiscoveryURL,
		}
		requirements = append(requirements, a2a.SecurityRequirements{oidcSecuritySchemeName: {}})
	}

	return &a2a.AgentCard{
		Name:        "Nightingale Agent",
		Description: desc,
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(endpointURL, a2a.TransportProtocolHTTPJSON),
		},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Capabilities: a2a.AgentCapabilities{
			Streaming: true,
		},
		SecuritySchemes:      schemes,
		SecurityRequirements: requirements,
		Skills:               skills,
	}
}

// loadBuiltinAgentSkills 把内置 skill 的 frontmatter 映射为 A2A AgentSkill。
// 数据源是 skillpkg.ListBuiltinFrontmatters() —— 进程内只解析一次的共享缓存，
// AgentCard 这条公开发现端点和 SkillRegistry 共用同一份元数据。
func loadBuiltinAgentSkills() []a2a.AgentSkill {
	metas := skillpkg.ListBuiltinFrontmatters()
	skills := make([]a2a.AgentSkill, 0, len(metas))
	for _, m := range metas {
		tags := m.Tags
		if len(tags) == 0 {
			tags = []string{"n9e"}
		}
		skills = append(skills, a2a.AgentSkill{
			ID:          m.Name,
			Name:        m.Name,
			Description: strings.TrimSpace(m.Description),
			Tags:        tags,
			Examples:    m.Examples,
		})
	}
	return skills
}
