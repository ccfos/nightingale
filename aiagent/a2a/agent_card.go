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
}

const securitySchemeName a2a.SecuritySchemeName = "x-user-token"

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

		card := buildAgentCard(base+path, opts.TokenHeaderName)
		a2asrv.NewStaticAgentCardHandler(card).ServeHTTP(w, r)
	})
}

func buildAgentCard(endpointURL, tokenHeader string) *a2a.AgentCard {
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
			Examples: []string{
				"查看 prod 业务组当前正在告警的事件",
				"对告警 #12345 做根因分析",
				"总结过去 1 小时 host-01 的指标异常",
				"帮我创建一个 CPU > 80% 的告警规则",
			},
		}}
	}

	return &a2a.AgentCard{
		Name:        "Nightingale Agent",
		Description: desc,
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(endpointURL, a2a.TransportProtocolHTTPJSON),
		},
		DefaultInputModes: []string{"text"},
		// "data" advertises that some skills emit structured Data parts
		// alongside text — alert rules and dashboards come back as JSON
		// payloads with vendor MIME types (application/vnd.n9e.alert-rule+json,
		// application/vnd.n9e.dashboard+json). Clients that don't recognise
		// the MIME still get the raw object and can ignore it.
		DefaultOutputModes: []string{"text", "data"},
		Capabilities: a2a.AgentCapabilities{
			Streaming: true,
		},
		SecuritySchemes: a2a.NamedSecuritySchemes{
			securitySchemeName: a2a.APIKeySecurityScheme{
				Description: "n9e user API token. Manage tokens under user profile in the n9e UI.",
				Location:    a2a.APIKeySecuritySchemeLocationHeader,
				Name:        tokenHeader,
			},
		},
		SecurityRequirements: a2a.SecurityRequirementsOptions{
			a2a.SecurityRequirements{securitySchemeName: {}},
		},
		Skills: skills,
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
