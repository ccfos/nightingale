// Package agentassets holds the build-time-pinned collector metadata and the
// install-script template, embedded into the binary so they stay correct even
// in deployments where the on-disk agents/ directory was stripped.
package agentassets

import (
	_ "embed"
	"strings"
)

// categrafVersionRaw is the single source of truth for which categraf release
// is bundled. scripts/download_categraf.sh reads the SAME file, so the tarball
// staged on disk and the version this binary reports can never drift apart.
// Bumping categraf is a one-line edit to categraf.version.
//
//go:embed categraf.version
var categrafVersionRaw string

// InstallCategrafTpl is the install script served by
// GET /api/n9e/agents/categraf/install.sh. It is a text/template (NOT
// html/template — HTML escaping would corrupt shell syntax).
//
//go:embed install-categraf.sh.tmpl
var InstallCategrafTpl string

// CategrafVersion returns the pinned categraf release tag, e.g. "v0.5.15".
func CategrafVersion() string {
	return strings.TrimSpace(categrafVersionRaw)
}
