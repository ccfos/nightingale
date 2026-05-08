package a2a

import "github.com/ccfos/nightingale/v6/models"

// artifactKind is the short tag we put in StreamMessage envelopes and Part
// metadata to identify a structured (non-text) payload. The set is closed
// and grows by adding entries to the lookup tables below — producer
// (router_ai_assistant.go), bridge real-time path, and Finalize safety net
// all key off these tables, so a new kind only takes one place to register.
type artifactKind string

const (
	kindAlertRule  artifactKind = "alert_rule"
	kindDashboard  artifactKind = "dashboard"
	kindFormSelect artifactKind = "form_select"
)

// vendorMime maps a kind to the vendor-specific MIME that travels with the
// A2A Data part. Clients that don't know the MIME still get the raw JSON
// content; clients that do can render a typed card.
func vendorMime(k artifactKind) string {
	switch k {
	case kindAlertRule:
		return "application/vnd.n9e.alert-rule+json"
	case kindDashboard:
		return "application/vnd.n9e.dashboard+json"
	case kindFormSelect:
		return "application/vnd.n9e.form-select+json"
	}
	return ""
}

// kindForTool returns the artifact kind a successful tool invocation should
// emit, or "" when the tool's output is purely informational (no structured
// card to surface). Used by the producer side in processAssistantMessage to
// decide whether to push a P:artifact envelope into streamBus.
func kindForTool(tool string) artifactKind {
	switch tool {
	case "create_alert_rule":
		return kindAlertRule
	case "create_dashboard":
		return kindDashboard
	}
	return ""
}

// ArtifactKindForTool is the exported, string-typed wrapper for kindForTool —
// the producer side (center/router) calls this to decide whether to push a
// structured artifact envelope into streamBus for a given tool's result.
// Returns "" for tools whose output is text-only.
func ArtifactKindForTool(tool string) string {
	return string(kindForTool(tool))
}

// VendorMimeForKind is the exported wrapper for vendorMime, used by the
// producer to fill the envelope's mime field at emission time so the wire
// payload is self-describing.
func VendorMimeForKind(kind string) string {
	return vendorMime(artifactKind(kind))
}

// kindForContentType maps a stored AssistantMessage.Response ContentType to
// an artifact kind. Used by the bridge.Finalize safety net to translate
// Response items that never went through streamBus (notably the halted-flow
// finishHaltedMessage path that returns form_select payloads directly).
//
// Returning "" means "this ContentType is text-y and was already emitted via
// the P:content stream — skip it in the safety net."
func kindForContentType(ct models.AssistantContentType) artifactKind {
	switch ct {
	case models.ContentTypeAlertRule:
		return kindAlertRule
	case models.ContentTypeDashboard:
		return kindDashboard
	case models.ContentTypeFormSelect:
		return kindFormSelect
	}
	return ""
}
