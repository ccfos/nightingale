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
