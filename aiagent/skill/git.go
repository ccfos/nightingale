package skill

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

const (
	GitRefBranch = "branch"
	GitRefTag    = "tag"
	GitRefCommit = "commit"

	GitAuthNone  = "none"
	GitAuthToken = "token"

	defaultGitAuthUsername = "token"
)

// GitConfig describes a Git-backed Skill source.
type GitConfig struct {
	URL      string
	RefType  string
	Ref      string
	AuthType string
	Token    string
	Subdir   string
}

type GitFetchResult struct {
	Meta         Frontmatter
	Instructions string
	Files        map[string]string
	Commit       string
}

func (c *GitConfig) Validate(requireToken bool) error {
	if c.URL == "" {
		return fmt.Errorf("git_url is required")
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("invalid git_url: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("git_url must be an http or https URL")
	}
	switch c.RefType {
	case GitRefBranch, GitRefTag, GitRefCommit:
	default:
		return fmt.Errorf("git_ref_type must be one of branch/tag/commit")
	}
	if c.Ref == "" {
		return fmt.Errorf("git_ref is required")
	}
	switch c.AuthType {
	case GitAuthNone:
	case GitAuthToken:
		if requireToken && c.Token == "" {
			return fmt.Errorf("git_token is required when git_auth_type=token")
		}
	default:
		return fmt.Errorf("git_auth_type must be one of none/token")
	}
	if _, err := cleanGitSubdir(c.Subdir); err != nil {
		return err
	}
	return nil
}

func gitAuth(c GitConfig) transport.AuthMethod {
	if c.AuthType != GitAuthToken || c.Token == "" {
		return nil
	}
	username, token := gitAuthCredential(c.Token)
	return &githttp.BasicAuth{
		Username: username,
		Password: token,
	}
}

func gitAuthCredential(token string) (string, string) {
	username := defaultGitAuthUsername
	password := token
	if u, p, ok := strings.Cut(token, ":"); ok {
		if u = strings.TrimSpace(u); u != "" {
			username = u
		}
		password = p
	}
	return username, password
}

func FetchGitSkill(ctx context.Context, cfg GitConfig) (*GitFetchResult, error) {
	if err := cfg.Validate(true); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "skill-git-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	repo, err := gogit.PlainCloneContext(ctx, tmpDir, false, &gogit.CloneOptions{
		URL:             cfg.URL,
		Auth:            gitAuth(cfg),
		Tags:            gogit.AllTags,
		InsecureSkipTLS: true,
	})
	if err != nil {
		return nil, fmt.Errorf("clone git repository: %w", err)
	}

	hash, err := resolveGitCommit(repo, cfg)
	if err != nil {
		return nil, err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open git worktree: %w", err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: hash, Force: true}); err != nil {
		return nil, fmt.Errorf("checkout %s %q: %w", cfg.RefType, cfg.Ref, err)
	}

	root := tmpDir
	if subdir, err := cleanGitSubdir(cfg.Subdir); err != nil {
		return nil, err
	} else if subdir != "" {
		root = filepath.Join(tmpDir, subdir)
	}

	files, err := Walk(root)
	if err != nil {
		return nil, err
	}
	skillMD, ok := files["SKILL.md"]
	if !ok || strings.TrimSpace(skillMD) == "" {
		return nil, fmt.Errorf("SKILL.md not found in git source root")
	}
	meta, instructions, ok := ParseMarkdown(skillMD)
	if !ok {
		return nil, fmt.Errorf("SKILL.md must contain valid YAML frontmatter with a non-empty 'name' field")
	}
	if strings.TrimSpace(instructions) == "" {
		return nil, fmt.Errorf("instructions is required")
	}

	return &GitFetchResult{
		Meta:         meta,
		Instructions: instructions,
		Files:        files,
		Commit:       hash.String(),
	}, nil
}

func LatestGitCommit(ctx context.Context, cfg GitConfig) (string, error) {
	if err := cfg.Validate(true); err != nil {
		return "", err
	}
	if cfg.RefType == GitRefCommit {
		return cfg.Ref, nil
	}

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{cfg.URL},
	})
	refs, err := remote.ListContext(ctx, &gogit.ListOptions{
		Auth:            gitAuth(cfg),
		InsecureSkipTLS: true,
	})
	if err != nil {
		return "", fmt.Errorf("list remote refs: %w", err)
	}

	switch cfg.RefType {
	case GitRefBranch:
		name := plumbing.NewBranchReferenceName(cfg.Ref)
		for _, ref := range refs {
			if ref.Name() == name {
				return ref.Hash().String(), nil
			}
		}
		return "", fmt.Errorf("remote branch %q not found", cfg.Ref)
	case GitRefTag:
		peeledName := plumbing.ReferenceName("refs/tags/" + cfg.Ref + "^{}")
		tagName := plumbing.NewTagReferenceName(cfg.Ref)
		var tagHash string
		for _, ref := range refs {
			if ref.Name() == peeledName {
				return ref.Hash().String(), nil
			}
			if ref.Name() == tagName {
				tagHash = ref.Hash().String()
			}
		}
		if tagHash != "" {
			return tagHash, nil
		}
		return "", fmt.Errorf("remote tag %q not found", cfg.Ref)
	default:
		return "", fmt.Errorf("git_ref_type must be one of branch/tag/commit")
	}
}

func resolveGitCommit(repo *gogit.Repository, cfg GitConfig) (plumbing.Hash, error) {
	switch cfg.RefType {
	case GitRefBranch:
		names := []plumbing.ReferenceName{
			plumbing.NewRemoteReferenceName("origin", cfg.Ref),
			plumbing.NewBranchReferenceName(cfg.Ref),
		}
		for _, name := range names {
			ref, err := repo.Reference(name, true)
			if err == nil {
				return ref.Hash(), nil
			}
		}
		return plumbing.ZeroHash, fmt.Errorf("branch %q not found", cfg.Ref)
	case GitRefTag:
		ref, err := repo.Reference(plumbing.NewTagReferenceName(cfg.Ref), false)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("tag %q not found: %w", cfg.Ref, err)
		}
		hash := ref.Hash()
		if tag, err := repo.TagObject(hash); err == nil {
			return tag.Target, nil
		}
		return hash, nil
	case GitRefCommit:
		hash, err := repo.ResolveRevision(plumbing.Revision(cfg.Ref))
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("commit %q not found: %w", cfg.Ref, err)
		}
		return *hash, nil
	default:
		return plumbing.ZeroHash, fmt.Errorf("git_ref_type must be one of branch/tag/commit")
	}
}

func cleanGitSubdir(subdir string) (string, error) {
	s := strings.TrimSpace(subdir)
	if s == "" {
		return "", nil
	}
	s = filepath.ToSlash(s)
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("git_subdir must be a relative path")
	}
	cleaned := filepath.ToSlash(filepath.Clean(s))
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("git_subdir escapes repository root")
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return "", fmt.Errorf("git_subdir escapes repository root")
		}
	}
	return cleaned, nil
}
