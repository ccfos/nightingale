package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v3"
)

// DBSkill is the source-of-truth snapshot of a single DB skill that the sync
// needs to materialize. Defined here (not in `models`) so the skill package
// stays free of DB imports — the caller adapts gorm rows into this shape.
type DBSkill struct {
	Name          string
	Description   string
	Instructions  string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  string
	Files         []DBSkillFile
}

// DBSkillFile is a single attached file under a DB skill. Name is the relative
// path inside the skill directory (e.g. "skill_tools/foo.yaml"); Content is raw
// bytes-as-string (matches the `mediumtext` column type in ai_skill_file).
type DBSkillFile struct {
	Name    string
	Content string
}

// SyncDBSkills materializes `skills` under skillsPath, enforcing the invariant
// that every DB-backed directory carries a `.fromdb` marker (see FromDBMarker).
//
// Contract:
//   - Creates skillsPath if missing.
//   - For each skill: writes .fromdb first, then SKILL.md (frontmatter + body),
//     then attached files under their relative paths. Existing file content is
//     overwritten; files that vanish from DB are removed (full sync per skill).
//   - Removes stale .fromdb directories whose name is no longer in `skills`.
//     Directories without .fromdb are left alone (ExtractBuiltin owns those).
//   - A DB skill whose name collides with a builtin wins: the builtin copy gets
//     displaced on the next ExtractBuiltin run (it skips when .fromdb exists).
//   - Partial failures: logged + joined via errors.Join. A single bad skill
//     does not abort the rest.
//
// Rejects any file name that escapes skillsPath via path traversal. The size
// limits on SKILL.md and individual files match the archive-import path so
// the DB round-trip can't smuggle larger payloads onto disk.
func SyncDBSkills(skillsPath string, skills []DBSkill) error {
	if skillsPath == "" {
		return fmt.Errorf("skillsPath is empty")
	}
	if err := os.MkdirAll(skillsPath, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", skillsPath, err)
	}

	abs, err := filepath.Abs(skillsPath)
	if err != nil {
		return fmt.Errorf("abs %s: %w", skillsPath, err)
	}
	skillsPath = abs

	wanted := make(map[string]struct{}, len(skills))
	for _, s := range skills {
		if s.Name == "" {
			continue
		}
		wanted[s.Name] = struct{}{}
	}

	var errs []error

	// Step 1: drop .fromdb directories that no longer have a DB backer.
	if existing, err := os.ReadDir(skillsPath); err == nil {
		for _, e := range existing {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(skillsPath, e.Name())
			if !IsFromDB(dir) {
				continue
			}
			if _, keep := wanted[e.Name()]; keep {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				logger.Warningf("remove orphan db skill dir %s failed: %v", dir, err)
				errs = append(errs, fmt.Errorf("remove orphan %s: %w", e.Name(), err))
			}
		}
	} else {
		errs = append(errs, fmt.Errorf("read skills dir: %w", err))
	}

	// Step 2: upsert each DB skill.
	for i := range skills {
		s := &skills[i]
		if s.Name == "" {
			continue
		}
		if err := writeOneSkill(skillsPath, s); err != nil {
			logger.Warningf("sync db skill %q failed: %v", s.Name, err)
			errs = append(errs, fmt.Errorf("sync %s: %w", s.Name, err))
		}
	}

	return errors.Join(errs...)
}

// SyncOneDBSkill is the single-skill hot path, invoked after CRUD. Avoids a
// full directory scan; the caller is responsible for deletion via
// RemoveOneDBSkill when a skill goes away.
func SyncOneDBSkill(skillsPath string, skill DBSkill) error {
	if skillsPath == "" {
		return fmt.Errorf("skillsPath is empty")
	}
	if skill.Name == "" {
		return fmt.Errorf("skill name is empty")
	}
	abs, err := filepath.Abs(skillsPath)
	if err != nil {
		return fmt.Errorf("abs %s: %w", skillsPath, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", abs, err)
	}
	return writeOneSkill(abs, &skill)
}

// RemoveOneDBSkill deletes the on-disk directory for a DB skill by name.
// Guards against accidentally removing a builtin skill by checking .fromdb.
func RemoveOneDBSkill(skillsPath, name string) error {
	if skillsPath == "" || name == "" {
		return nil
	}
	abs, err := filepath.Abs(skillsPath)
	if err != nil {
		return err
	}
	dir := filepath.Join(abs, name)
	if !IsFromDB(dir) {
		return nil
	}
	return os.RemoveAll(dir)
}

func writeOneSkill(skillsPath string, s *DBSkill) error {
	// Reject a name that would break out of skillsPath (`..`, absolute paths,
	// embedded separators). Names come from DB and pass Verify() at ingest,
	// but we treat skillsPath as a trust boundary and re-check here.
	if strings.ContainsAny(s.Name, `/\`) || s.Name == "." || s.Name == ".." {
		return fmt.Errorf("illegal skill name %q", s.Name)
	}

	dir := filepath.Join(skillsPath, s.Name)
	// Ensure the resolved path stays inside skillsPath (defence-in-depth vs. the
	// name check above; cheap to double up).
	relCheck, err := filepath.Rel(skillsPath, dir)
	if err != nil || strings.HasPrefix(relCheck, "..") {
		return fmt.Errorf("skill dir escapes skillsPath: %s", dir)
	}

	// Invariant: .fromdb is created BEFORE any content so a mid-write crash
	// leaves a marker-bearing (incomplete) dir that the next sync will
	// overwrite, rather than an unmarked dir that ExtractBuiltin would nuke.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := MarkFromDB(dir); err != nil {
		return fmt.Errorf("mark fromdb: %w", err)
	}

	// Write SKILL.md with frontmatter. yaml.Marshal produces deterministic
	// output (alphabetical keys), which keeps the on-disk bytes stable across
	// syncs — helpful for any downstream file-mtime-based cache.
	skillMD, err := buildSkillMD(s)
	if err != nil {
		return fmt.Errorf("build SKILL.md: %w", err)
	}
	if len(skillMD) > MaxSkillMD {
		return fmt.Errorf("SKILL.md exceeds %dKB limit (%d bytes)", MaxSkillMD/1024, len(skillMD))
	}
	if err := writeFileAtomic(filepath.Join(dir, "SKILL.md"), []byte(skillMD)); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}

	// Build the set of relative paths we're about to write, so we can prune
	// files that used to be there but no longer are.
	incoming := make(map[string]struct{}, len(s.Files))
	for _, f := range s.Files {
		rel, err := safeRelPath(f.Name)
		if err != nil {
			return fmt.Errorf("file %q: %w", f.Name, err)
		}
		if rel == "SKILL.md" || rel == FromDBMarker {
			// SKILL.md is managed separately; .fromdb is our marker.
			// Silently ignore (import step could have accidentally picked them up).
			continue
		}
		if int64(len(f.Content)) > MaxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", rel, MaxSingleFile/1024/1024, len(f.Content))
		}

		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(full), err)
		}
		if err := writeFileAtomic(full, []byte(f.Content)); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
		incoming[rel] = struct{}{}
	}

	// Prune stale files inside this skill directory (anything not in incoming,
	// not SKILL.md, not .fromdb). We intentionally walk after writes so a
	// mid-sync crash leaves the "extra" file as a no-op rather than a hole.
	if err := pruneStaleFiles(dir, incoming); err != nil {
		logger.Warningf("prune stale files in %s: %v", dir, err)
	}

	return nil
}

// buildSkillMD emits a round-trippable SKILL.md: the frontmatter uses the same
// keys markdown.Frontmatter parses, and empty fields are dropped so the output
// is clean rather than `license: ""` noise. We build from a local struct with
// omitempty tags (rather than mutating the shared Frontmatter type) so the
// parse-side stays permissive.
func buildSkillMD(s *DBSkill) (string, error) {
	fm := struct {
		Name          string            `yaml:"name"`
		Description   string            `yaml:"description,omitempty"`
		License       string            `yaml:"license,omitempty"`
		Compatibility string            `yaml:"compatibility,omitempty"`
		Metadata      map[string]string `yaml:"metadata,omitempty"`
		AllowedTools  string            `yaml:"allowed-tools,omitempty"`
	}{
		Name:          s.Name,
		Description:   s.Description,
		License:       s.License,
		Compatibility: s.Compatibility,
		Metadata:      s.Metadata,
		AllowedTools:  s.AllowedTools,
	}
	buf, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(buf)
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(s.Instructions))
	sb.WriteString("\n")
	return sb.String(), nil
}

// safeRelPath normalizes and validates a DB-supplied file name so it cannot
// escape its skill directory. Rejects absolute paths, `..` segments, and
// symlink-like tricks.
func safeRelPath(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", fmt.Errorf("empty")
	}
	// Normalize slashes; DB could hold either separator.
	n = filepath.ToSlash(n)
	if strings.HasPrefix(n, "/") {
		return "", fmt.Errorf("absolute path not allowed")
	}
	cleaned := filepath.ToSlash(filepath.Clean(n))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes skill dir")
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return "", fmt.Errorf("path escapes skill dir")
		}
	}
	return cleaned, nil
}

// pruneStaleFiles removes files under dir that are not in the incoming set.
// Preserves SKILL.md and the .fromdb marker unconditionally. Empty directories
// left behind are removed bottom-up.
func pruneStaleFiles(dir string, incoming map[string]struct{}) error {
	type entry struct {
		rel   string
		isDir bool
	}
	var all []entry
	err := filepath.Walk(dir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		all = append(all, entry{rel: filepath.ToSlash(rel), isDir: info.IsDir()})
		return nil
	})
	if err != nil {
		return err
	}
	// Sort descending so children are visited before parents — lets us remove
	// an empty directory after pruning its files.
	sort.Slice(all, func(i, j int) bool { return all[i].rel > all[j].rel })
	for _, e := range all {
		if e.rel == "SKILL.md" || e.rel == FromDBMarker {
			continue
		}
		full := filepath.Join(dir, e.rel)
		if e.isDir {
			entries, err := os.ReadDir(full)
			if err == nil && len(entries) == 0 {
				_ = os.Remove(full)
			}
			continue
		}
		if _, keep := incoming[e.rel]; keep {
			continue
		}
		_ = os.Remove(full)
	}
	return nil
}

// writeFileAtomic writes to a temp file in the same directory and renames.
// The rename is atomic on POSIX within a filesystem, which is the single-node
// guarantee we need (no cross-instance sync here).
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup if rename didn't happen.
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
