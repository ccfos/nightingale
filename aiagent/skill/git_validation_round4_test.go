package skill

import (
	"context"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/require"
)

func TestGitConfigValidate_RejectsInvalidSource(t *testing.T) {
	cases := []struct {
		name string
		edit func(*GitConfig)
		want string
	}{
		{name: "missing URL", edit: func(c *GitConfig) { c.URL = "" }, want: "git_url is required"},
		{name: "malformed URL", edit: func(c *GitConfig) { c.URL = "https://%zz" }, want: "invalid git_url"},
		{name: "missing host", edit: func(c *GitConfig) { c.URL = "https:///skills" }, want: "http or https"},
		{name: "unknown ref type", edit: func(c *GitConfig) { c.RefType = "head" }, want: "git_ref_type"},
		{name: "missing ref", edit: func(c *GitConfig) { c.Ref = "" }, want: "git_ref is required"},
		{name: "unknown auth", edit: func(c *GitConfig) { c.AuthType = "ssh" }, want: "git_auth_type"},
		{name: "missing token", edit: func(c *GitConfig) { c.AuthType = GitAuthToken }, want: "git_token is required"},
		{name: "absolute subdir", edit: func(c *GitConfig) { c.Subdir = "/skills" }, want: "relative path"},
		{name: "escaping subdir", edit: func(c *GitConfig) { c.Subdir = "../skills" }, want: "escapes repository root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := GitConfig{URL: "https://example.invalid/skills", RefType: GitRefBranch, Ref: "main", AuthType: GitAuthNone}
			tc.edit(&cfg)

			err := cfg.Validate(true)

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestGitAuth_UsesConfiguredBasicCredentials(t *testing.T) {
	cfg := GitConfig{AuthType: GitAuthToken, Token: "deploy:secret"}

	auth, ok := gitAuth(cfg).(*http.BasicAuth)

	require.True(t, ok)
	require.Equal(t, "deploy", auth.Username)
	require.Equal(t, "secret", auth.Password)
	require.Nil(t, gitAuth(GitConfig{AuthType: GitAuthNone, Token: "ignored"}))
	require.Nil(t, gitAuth(GitConfig{AuthType: GitAuthToken}))
}

func TestCleanGitSubdir_NormalizesRelativePaths(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: " ", want: ""},
		{input: ".", want: ""},
		{input: " ./skills/triage/ ", want: "skills/triage"},
		{input: "skills/old/../triage", want: "skills/triage"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := cleanGitSubdir(tc.input)

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLatestGitCommit_PinnedCommitNeedsNoRemote(t *testing.T) {
	cfg := GitConfig{
		URL: "https://example.invalid/skills", RefType: GitRefCommit,
		Ref: "0123456789012345678901234567890123456789", AuthType: GitAuthNone,
	}

	got, err := LatestGitCommit(context.Background(), cfg)

	require.NoError(t, err)
	require.Equal(t, cfg.Ref, got)
}

func TestResolveGitCommit_PrefersRemoteBranch(t *testing.T) {
	store := memory.NewStorage()
	repo, err := gogit.Init(store, nil)
	require.NoError(t, err)
	remote := plumbing.NewHash("1111111111111111111111111111111111111111")
	local := plumbing.NewHash("2222222222222222222222222222222222222222")
	remoteRef := plumbing.NewHashReference(plumbing.NewRemoteReferenceName("origin", "main"), remote)
	require.NoError(t, store.SetReference(remoteRef))
	require.NoError(t, store.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), local)))

	got, err := resolveGitCommit(repo, GitConfig{RefType: GitRefBranch, Ref: "main"})

	require.NoError(t, err)
	require.Equal(t, remote, got)
}

func TestResolveGitCommit_UsesLocalBranchFallback(t *testing.T) {
	store := memory.NewStorage()
	repo, err := gogit.Init(store, nil)
	require.NoError(t, err)
	want := plumbing.NewHash("2222222222222222222222222222222222222222")
	require.NoError(t, store.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), want)))

	got, err := resolveGitCommit(repo, GitConfig{RefType: GitRefBranch, Ref: "main"})

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestResolveGitCommit_ResolvesLightweightTag(t *testing.T) {
	store := memory.NewStorage()
	repo, err := gogit.Init(store, nil)
	require.NoError(t, err)
	want := plumbing.NewHash("3333333333333333333333333333333333333333")
	require.NoError(t, store.SetReference(plumbing.NewHashReference(plumbing.NewTagReferenceName("v1"), want)))

	got, err := resolveGitCommit(repo, GitConfig{RefType: GitRefTag, Ref: "v1"})

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestResolveGitCommit_RejectsMissingRefs(t *testing.T) {
	for _, refType := range []string{GitRefBranch, GitRefTag, GitRefCommit, "unknown"} {
		t.Run(refType, func(t *testing.T) {
			repo, err := gogit.Init(memory.NewStorage(), nil)
			require.NoError(t, err)

			got, err := resolveGitCommit(repo, GitConfig{RefType: refType, Ref: "missing"})

			require.Error(t, err)
			require.Equal(t, plumbing.ZeroHash, got)
		})
	}
}
