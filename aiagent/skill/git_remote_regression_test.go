package skill

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/stretchr/testify/require"
)

// Annotated tags regressed to returning the tag object hash instead of the peeled commit.
func TestLatestGitCommitAnnotatedTagRegression(t *testing.T) {
	tagHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	commitHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	adv := packp.NewAdvRefs()
	adv.Prefix = [][]byte{[]byte("# service=git-upload-pack\n"), nil}
	adv.References["refs/heads/main"] = commitHash
	adv.References["refs/tags/v1"] = tagHash
	adv.Peeled["refs/tags/v1"] = commitHash
	var payload bytes.Buffer
	require.NoError(t, adv.Encode(&payload))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		_, _ = w.Write(payload.Bytes())
	}))
	t.Cleanup(upstream.Close)

	got, err := LatestGitCommit(context.Background(), GitConfig{
		URL: upstream.URL + "/skills.git", RefType: GitRefTag, Ref: "v1", AuthType: GitAuthNone,
	})

	require.NoError(t, err)
	require.Equal(t, commitHash.String(), got)
}

// Remote listing errors regressed to exposing credentials embedded in the underlying error URL.
func TestLatestGitCommitErrorCredentialsRegression(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "repository unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	remoteURL, err := url.Parse(upstream.URL + "/skills.git")
	require.NoError(t, err)
	remoteURL.User = url.UserPassword("alice", "secret")

	_, err = LatestGitCommit(context.Background(), GitConfig{
		URL: remoteURL.String(), RefType: GitRefBranch, Ref: "main", AuthType: GitAuthNone,
	})

	require.Error(t, err)
	require.NotContains(t, err.Error(), "alice")
	require.NotContains(t, err.Error(), "secret")
}
