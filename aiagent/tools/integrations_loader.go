package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

// integrations/ 目录下每个组件是 categraf 配置语法和指标命名的权威 ground truth
// （比 flashcat.cloud/index.json 上的散文文档更可信）。
// 把 markdown/README.md 和 collect/*/*.toml 转成 docEntry 并入索引,
// LLM 在 doc_qa 流程里就能直接搜到真实的 [[instances]] 写法。
//
// 路径策略跟 center/integration/init.go 保持一致, 都是相对 runner.Cwd 找 integrations/.

const (
	// 标题前缀用作 source 标记, 让 LLM 和 scoreDocEntry 都能识别条目类型
	integrationConfigTitlePrefix = "[integration-config] "
	integrationDocTitlePrefix    = "[integration-doc] "
)

// loadIntegrationsEntries scans the integrations/ tree and emits docEntries
// for each component's README.md (one entry) and each collect/*/*.toml file
// (one entry per file). Returns nil if integrations/ is missing — caller
// (refreshDocIndex) treats that as "no extras", remote index alone keeps working.
func loadIntegrationsEntries() ([]docEntry, error) {
	dir := findIntegrationsDir()
	if dir == "" {
		return nil, nil
	}

	components, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read integrations dir %s: %w", dir, err)
	}

	var entries []docEntry
	skipped := 0
	for _, c := range components {
		if !c.IsDir() {
			continue
		}
		extras, err := scanIntegrationComponent(dir, c.Name())
		if err != nil {
			logger.Warningf("integrations: scan component %s: %v", c.Name(), err)
			skipped++
			continue
		}
		entries = append(entries, extras...)
	}
	logger.Infof("integrations: loaded %d entries from %d components (skipped %d) at %s",
		len(entries), len(components)-skipped, skipped, dir)
	return entries, nil
}

// findIntegrationsDir mirrors center/integration/init.go:50 so AI doc QA and
// the integration center read from the same source of truth. Returns "" if
// no candidate path exists — handled gracefully upstream.
func findIntegrationsDir() string {
	candidates := []string{
		filepath.Join(runner.Cwd, "integrations"),
		"integrations",
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

// scanIntegrationComponent emits up to (1 README + N toml) entries for one
// component dir. Other subdirs (icon/dashboards/alerts/metrics) are skipped
// for now — phase 1 only covers configuration ground truth.
func scanIntegrationComponent(root, name string) ([]docEntry, error) {
	base := filepath.Join(root, name)
	var entries []docEntry

	// 1) README.md → one [integration-doc] entry
	readmePath := filepath.Join(base, "markdown", "README.md")
	if data, err := os.ReadFile(readmePath); err == nil {
		entries = append(entries, docEntry{
			Title:       integrationDocTitlePrefix + name,
			Permalink:   ghBlobURL(name, "markdown/README.md"),
			Description: firstNonEmptyParagraph(string(data), 200),
			Contents:    string(data),
		})
	}

	// 2) collect/*/*.toml(.example) → one [integration-config] entry per file
	collectRoot := filepath.Join(base, "collect")
	if info, err := os.Stat(collectRoot); err == nil && info.IsDir() {
		err := filepath.WalkDir(collectRoot, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil // best-effort: skip unreadable files
			}
			if d.IsDir() {
				return nil
			}
			n := d.Name()
			if !strings.HasSuffix(n, ".toml") && !strings.HasSuffix(n, ".toml.example") {
				return nil
			}
			data, readErr := os.ReadFile(p)
			if readErr != nil {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			entries = append(entries, docEntry{
				Title:       integrationConfigTitlePrefix + name + " · " + n,
				Permalink:   ghBlobURL(name, filepath.ToSlash(rel)),
				Description: tomlLeadingComments(string(data), 200),
				// Wrap in fenced code block so the LLM treats it as a config sample
				// (not prose to paraphrase). The fence is part of contents so it
				// shows up directly when search_n9e_docs returns this entry.
				Contents: "```toml\n" + string(data) + "\n```",
			})
			return nil
		})
		if err != nil {
			logger.Warningf("integrations: walk %s: %v", collectRoot, err)
		}
	}

	return entries, nil
}

// ghBlobURL returns a stable github.com permalink that the LLM can cite.
// Tracks the upstream repo; if the user is on a fork they may want to
// override this in the future via a config knob.
func ghBlobURL(component, relFromComponent string) string {
	return "https://github.com/ccfos/nightingale/blob/main/integrations/" +
		component + "/" + relFromComponent
}

// firstNonEmptyParagraph extracts a short, readable description from a markdown
// README — skips heading lines so the description doesn't start with "# Title".
// maxRunes caps the result so docEntry.Description stays small.
func firstNonEmptyParagraph(md string, maxRunes int) string {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue // skip markdown headings
		}
		if strings.HasPrefix(t, "![") || strings.HasPrefix(t, "<!--") {
			continue
		}
		return truncateRunes(t, maxRunes)
	}
	return ""
}

// tomlLeadingComments returns the contiguous leading comment block of a toml
// file as a description hint. Catraf samples typically start with a few "# 用途"
// comments which are perfect 1-line descriptions.
func tomlLeadingComments(toml string, maxRunes int) string {
	var sb strings.Builder
	for _, line := range strings.Split(toml, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			if sb.Len() > 0 {
				break // blank line ends the leading comment block
			}
			continue
		}
		if !strings.HasPrefix(t, "#") {
			break
		}
		comment := strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if comment == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(comment)
	}
	return truncateRunes(sb.String(), maxRunes)
}
