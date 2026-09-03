package skillruntime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
)

// entryInfo is the inferred runtime for a skill (§11.1): which script to run and
// which interpreter, derived purely by convention — skill authors write normal
// scripts and declare nothing.
type entryInfo struct {
	Rel    string // entry path relative to the skill dir, e.g. "main.py"
	Interp string // "python3" | "bash"
	Type   string // "python" | "bash"
}

// validateSkillName rejects names that could escape the skills root (same gate
// the DB→FS materializer applies, dbsync.writeOneSkill).
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name is empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("illegal skill name %q", name)
	}
	return nil
}

// resolveEntry decides the entry script + interpreter. An explicit entry wins
// (must exist and have a known suffix); otherwise main.py → python, main.sh →
// bash, else the sole top-level .py/.sh. Anything ambiguous is an error so the
// caller can ask for an explicit entry rather than guess wrong.
func resolveEntry(skillDir, explicit string) (entryInfo, error) {
	if strings.TrimSpace(explicit) != "" {
		rel, err := relUnder(skillDir, explicit)
		if err != nil {
			return entryInfo{}, err
		}
		full := filepath.Join(skillDir, rel)
		if !fileExists(full) {
			return entryInfo{}, fmt.Errorf("entry %q not found in skill", explicit)
		}
		return inferInterp(rel)
	}

	for _, cand := range []string{"main.py", "main.sh"} {
		if fileExists(filepath.Join(skillDir, cand)) {
			return inferInterp(cand)
		}
	}

	// Fall back to a single top-level script.
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return entryInfo{}, fmt.Errorf("read skill dir: %w", err)
	}
	var scripts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".py") || strings.HasSuffix(n, ".sh") {
			scripts = append(scripts, n)
		}
	}
	switch len(scripts) {
	case 1:
		return inferInterp(scripts[0])
	case 0:
		return entryInfo{}, fmt.Errorf("skill has no runnable script (expected main.py / main.sh or a single .py/.sh); add one or pass entry")
	default:
		return entryInfo{}, fmt.Errorf("skill has multiple scripts %v; pass entry to pick one", scripts)
	}
}

func inferInterp(rel string) (entryInfo, error) {
	switch {
	case strings.HasSuffix(rel, ".py"):
		return entryInfo{Rel: rel, Interp: "python3", Type: "python"}, nil
	case strings.HasSuffix(rel, ".sh"):
		return entryInfo{Rel: rel, Interp: "bash", Type: "bash"}, nil
	}
	return entryInfo{}, fmt.Errorf("unsupported entry %q (only .py and .sh)", rel)
}

// stageSkillFiles copies the materialized skill files into dest (the workspace
// skill/ dir), skipping the .fromdb marker. Untrusted relative names are routed
// through sandbox.SafeJoin so none can escape dest.
func stageSkillFiles(skillDir, dest string) error {
	return filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == skill.FromDBMarker {
			return nil
		}
		target, err := sandbox.SafeJoin(dest, rel)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// relUnder cleans rel and confirms it stays under root (no absolute/.. escape).
func relUnder(root, rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return "", fmt.Errorf("empty entry")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("absolute entry not allowed")
	}
	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("entry escapes skill dir")
	}
	return cleaned, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
