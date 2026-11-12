package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/stretchr/testify/require"
)

// serveSkillRepository implements only the two smart-HTTP exchanges used by a clone.
// The repository and all HTTP traffic stay inside this test's temporary directory and loopback server.
func serveSkillRepository(t *testing.T, files map[string]string) (GitConfig, string, <-chan http.Header) {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "skills.git")
	repo, err := gogit.PlainInit(repoDir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeImportFile(t, repoDir, name, files[name])
		_, err := wt.Add(name)
		require.NoError(t, err)
	}
	hash, err := wt.Commit("skill fixture", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Unix(1700000000, 0)},
	})
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	endpoint, err := transport.NewEndpoint("file:///skills.git")
	require.NoError(t, err)
	backend := server.NewServer(server.MapLoader{endpoint.String(): repo.Storer})
	received := make(chan http.Header, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		session, err := backend.NewUploadPackSession(endpoint, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer session.Close()
		switch r.Method + " " + r.URL.RequestURI() {
		case "GET /skills.git/info/refs?service=git-upload-pack":
			refs, err := session.AdvertisedReferencesContext(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			refs.Prefix = [][]byte{[]byte("# service=git-upload-pack\n"), nil}
			w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
			if err := refs.Encode(w); err != nil {
				t.Errorf("encode advertised refs: %v", err)
			}
		case "POST /skills.git/git-upload-pack":
			request := packp.NewUploadPackRequest()
			if err := request.Decode(r.Body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			response, err := session.UploadPack(r.Context(), request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer response.Close()
			w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
			if err := response.Encode(w); err != nil {
				t.Errorf("encode upload pack: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	return GitConfig{
		URL: upstream.URL + "/skills.git", RefType: GitRefBranch, Ref: head.Name().Short(), AuthType: GitAuthNone,
	}, hash.String(), received
}

func TestFetchGitSkill_ReadsSubdirectoryAndAuthenticates(t *testing.T) {
	md := "---\nname: triage\ndescription: Inspect CPU\nbuiltin_tools: [list_targets]\n---\nRead the metrics.\n"
	cfg, commit, received := serveSkillRepository(t, map[string]string{
		"README.md": "outside the skill", "triage/SKILL.md": md, "triage/references/guide.md": "runbook",
	})
	cfg.Subdir, cfg.AuthType, cfg.Token = "./triage/", GitAuthToken, "deploy:secret"

	got, err := FetchGitSkill(context.Background(), cfg)

	require.NoError(t, err)
	require.Equal(t, commit, got.Commit)
	require.Equal(t, "triage", got.Meta.Name)
	require.Equal(t, "Inspect CPU", got.Meta.Description)
	require.Equal(t, []string{"list_targets"}, got.Meta.BuiltinTools)
	require.Equal(t, "Read the metrics.", got.Instructions)
	require.Equal(t, map[string]string{"SKILL.md": md, "references/guide.md": "runbook"}, got.Files)
	// Both ref discovery and pack download must carry the configured credentials.
	for i := 0; i < 2; i++ {
		request := &http.Request{Header: <-received}
		username, password, ok := request.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "deploy", username)
		require.Equal(t, "secret", password)
	}
}

func TestFetchGitSkill_RejectsInvalidSkillContent(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "missing instructions file", files: map[string]string{"README.md": "not a skill"}, want: "SKILL.md not found"},
		{name: "blank instructions file", files: map[string]string{"SKILL.md": " \n"}, want: "SKILL.md not found"},
		{name: "missing frontmatter", files: map[string]string{"SKILL.md": "Read metrics."}, want: "valid YAML frontmatter"},
		{
			name: "empty body", files: map[string]string{"SKILL.md": "---\nname: triage\n---\n \n"},
			want: "instructions is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, _ := serveSkillRepository(t, tc.files)

			got, err := FetchGitSkill(context.Background(), cfg)

			require.Nil(t, got)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestFetchGitSkill_ReportsMissingRef(t *testing.T) {
	cfg, _, _ := serveSkillRepository(t, map[string]string{"SKILL.md": "---\nname: triage\n---\nRead metrics."})
	cfg.Ref = "absent"

	got, err := FetchGitSkill(context.Background(), cfg)

	require.Nil(t, got)
	require.EqualError(t, err, `branch "absent" not found`)
}

func TestFetchGitSkill_ReportsMissingSubdirectory(t *testing.T) {
	cfg, _, _ := serveSkillRepository(t, map[string]string{"SKILL.md": "---\nname: triage\n---\nRead metrics."})
	cfg.Subdir = "absent"

	got, err := FetchGitSkill(context.Background(), cfg)

	require.Nil(t, got)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestFetchGitSkill_ValidatesBeforeContactingSource(t *testing.T) {
	got, err := FetchGitSkill(context.Background(), GitConfig{})

	require.Nil(t, got)
	require.EqualError(t, err, "git_url is required")
}

func TestFetchGitSkill_ReportsCloneFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	cfg := GitConfig{URL: upstream.URL + "/skills.git", RefType: GitRefBranch, Ref: "main", AuthType: GitAuthNone}

	got, err := FetchGitSkill(context.Background(), cfg)

	require.Nil(t, got)
	require.ErrorContains(t, err, "clone git repository url="+cfg.URL)
}
