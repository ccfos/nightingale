package skill

import "testing"

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
