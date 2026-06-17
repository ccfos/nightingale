package skill

import (
	"strings"
	"testing"
)

func TestGitAuthCredential(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		wantUsername string
		wantPassword string
	}{
		{
			name:         "plain token uses default username",
			token:        "github_pat_xxx",
			wantUsername: defaultGitAuthUsername,
			wantPassword: "github_pat_xxx",
		},
		{
			name:         "deploy token can include username",
			token:        "gitlab+deploy-token-1:gldt_xxx",
			wantUsername: "gitlab+deploy-token-1",
			wantPassword: "gldt_xxx",
		},
		{
			name:         "empty username falls back to default",
			token:        ":gldt_xxx",
			wantUsername: defaultGitAuthUsername,
			wantPassword: "gldt_xxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUsername, gotPassword := gitAuthCredential(tt.token)
			if gotUsername != tt.wantUsername {
				t.Fatalf("username expected %q, got %q", tt.wantUsername, gotUsername)
			}
			if gotPassword != tt.wantPassword {
				t.Fatalf("password expected %q, got %q", tt.wantPassword, gotPassword)
			}
		})
	}
}

func TestRedactedGitURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		notWant   []string
		wantParts []string
	}{
		{
			name:      "redacts username and password",
			rawURL:    "https://alice:secret@git.example.com/group/skills.git",
			notWant:   []string{"alice", "secret"},
			wantParts: []string{"https://xxxxx@git.example.com/group/skills.git"},
		},
		{
			name:      "redacts token-only userinfo",
			rawURL:    "https://github_pat_xxx@git.example.com/group/skills.git",
			notWant:   []string{"github_pat_xxx"},
			wantParts: []string{"https://xxxxx@git.example.com/group/skills.git"},
		},
		{
			name:      "keeps url without userinfo",
			rawURL:    "https://git.example.com/group/skills.git",
			wantParts: []string{"https://git.example.com/group/skills.git"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactedGitURL(tt.rawURL)
			for _, s := range tt.notWant {
				if strings.Contains(got, s) {
					t.Fatalf("expected %q to be redacted from %q", s, got)
				}
			}
			for _, s := range tt.wantParts {
				if !strings.Contains(got, s) {
					t.Fatalf("expected %q to contain %q", got, s)
				}
			}
		})
	}
}

func TestGitConfigValidateAllowsHTTPAndHTTPS(t *testing.T) {
	base := GitConfig{
		RefType:  GitRefBranch,
		Ref:      "main",
		AuthType: GitAuthNone,
	}

	for _, url := range []string{
		"http://git.example.com/group/skills.git",
		"https://git.example.com/group/skills.git",
	} {
		t.Run(url, func(t *testing.T) {
			cfg := base
			cfg.URL = url
			if err := cfg.Validate(false); err != nil {
				t.Fatalf("expected %s to be valid, got %v", url, err)
			}
		})
	}
}

func TestGitConfigValidateRejectsUnsupportedScheme(t *testing.T) {
	cfg := GitConfig{
		URL:      "ssh://git.example.com/group/skills.git",
		RefType:  GitRefBranch,
		Ref:      "main",
		AuthType: GitAuthNone,
	}

	if err := cfg.Validate(false); err == nil {
		t.Fatalf("expected unsupported scheme to be rejected")
	} else if err.Error() != "git_url must be an http or https URL" {
		t.Fatalf("unexpected error: %v", err)
	}
}
