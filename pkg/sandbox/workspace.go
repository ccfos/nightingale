package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is the per-execution on-disk layout (§9.1). The control plane
// creates it; the caller (skill_runtime) populates Skill/Input and reads Output.
// The canonical contract: skill(ro), input(ro), workspace(rw), output(rw).
type Workspace struct {
	Root      string // sessions/<exec_id>
	Skill     string // ro: materialized skill files
	Input     string // ro: platform-provided input
	Workspace string // rw: working dir (becomes ExecSpec.Cwd)
	Output    string // rw: artifacts
}

// NewWorkspace creates the directory tree for execID under the configured data
// dir. The caller must Cleanup() it when the run completes.
func (s *Sandbox) NewWorkspace(execID string) (*Workspace, error) {
	if strings.ContainsAny(execID, `/\`) || execID == "" || execID == "." || execID == ".." {
		return nil, fmt.Errorf("illegal exec id %q", execID)
	}
	root := filepath.Join(s.cfg.DataDir, "sessions", execID)
	w := &Workspace{
		Root:      root,
		Skill:     filepath.Join(root, "skill"),
		Input:     filepath.Join(root, "input"),
		Workspace: filepath.Join(root, "workspace"),
		Output:    filepath.Join(root, "output"),
	}
	for _, d := range []string{w.Skill, w.Input, w.Workspace, w.Output} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("create workspace %s: %w", d, err)
		}
	}
	return w, nil
}

// Cleanup removes the whole workspace tree. Best-effort; logs nothing so the
// caller controls error handling.
func (w *Workspace) Cleanup() {
	if w == nil || w.Root == "" {
		return
	}
	_ = os.RemoveAll(w.Root)
}

// SafeJoin joins rel under root, rejecting any result that escapes root via
// "..", absolute paths, or NUL. It is the single path-cleaning gate for copying
// untrusted skill file names into the sandbox tree (mirrors the dbsync guard).
func SafeJoin(root, rel string) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("path contains NUL")
	}
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %q", rel)
	}
	full := filepath.Join(root, cleaned)
	// Defence in depth: confirm the resolved path stays within root.
	relCheck, err := filepath.Rel(root, full)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %q", rel)
	}
	return full, nil
}
