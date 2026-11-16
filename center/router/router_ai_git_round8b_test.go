package router

import (
	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func r8bString(s string) *string { return &s }
func TestR8bGitConfigUpdates(t *testing.T) {
	rt := &Router{}
	current := &models.AISkill{GitInfo: &models.AISkillGitInfo{URL: "https://example.invalid/repo.git", RefType: skill.GitRefBranch, Ref: "main", AuthType: skill.GitAuthToken, Token: "legacy-token", Subdir: "skills/demo"}}
	cfg, fields, err := rt.gitConfigForUpdate(current, aiSkillGitRequest{}, false)
	require.NoError(t, err)
	require.Equal(t, "legacy-token", cfg.Token)
	require.Equal(t, "legacy-token", fields.Token)
	req := aiSkillGitRequest{GitURL: r8bString("https://example.invalid/new.git"), GitRefType: r8bString(skill.GitRefTag), GitRef: r8bString("v1"), GitAuthType: r8bString(skill.GitAuthNone), GitSubdir: r8bString("new")}
	cfg, fields, err = rt.gitConfigForUpdate(current, req, false)
	require.NoError(t, err)
	require.Equal(t, *req.GitURL, cfg.URL)
	require.Equal(t, "v1", fields.Ref)
	require.Empty(t, fields.Token)
	require.Equal(t, "new", fields.Subdir)
	cfg, fields, err = rt.gitConfigForUpdate(current, req, true)
	require.NoError(t, err)
	require.Equal(t, current.GitInfo.URL, cfg.URL)
	require.Equal(t, "main", fields.Ref)
	require.Equal(t, "legacy-token", fields.Token)
	_, _, err = rt.gitConfigForUpdate(current, aiSkillGitRequest{GitToken: r8bString("new-token")}, false)
	require.ErrorContains(t, err, "rsa public key")
	current.GitInfo.Token = "enc:broken"
	_, _, err = rt.gitConfigForUpdate(current, aiSkillGitRequest{}, false)
	require.ErrorContains(t, err, "rsa private key")
	_, _, err = rt.gitConfigForUpdate(current, aiSkillGitRequest{GitToken: r8bString("enc:broken")}, false)
	require.ErrorContains(t, err, "rsa private key")
	_, _, err = rt.gitConfigForUpdate(&models.AISkill{}, aiSkillGitRequest{}, false)
	require.Error(t, err)
}
func TestR8bGitInstallValidation(t *testing.T) {
	rt := &Router{}
	for _, tc := range []struct {
		req  aiSkillGitRequest
		want string
	}{
		{aiSkillGitRequest{GitAuthType: r8bString("token")}, "git_token is required"},
		{aiSkillGitRequest{GitAuthType: r8bString("token"), GitToken: r8bString("enc:broken")}, "rsa private key"},
		{aiSkillGitRequest{GitAuthType: r8bString("token"), GitToken: r8bString("plain")}, "rsa public key"},
		{aiSkillGitRequest{}, "git_url"},
	} {
		_, _, err := rt.gitConfigForInstall(tc.req)
		require.ErrorContains(t, err, tc.want)
	}
	cfg, fields, err := rt.gitConfigForInstall(aiSkillGitRequest{GitURL: r8bString("https://example.invalid/repo.git"), GitRefType: r8bString("branch"), GitRef: r8bString("main")})
	require.NoError(t, err)
	require.Equal(t, skill.GitAuthNone, cfg.AuthType)
	require.Equal(t, cfg.URL, fields.URL)
	for _, v := range []string{"", "  ", " plain ", "enc:stored"} {
		got, err := rt.encryptGitToken(v)
		if v == " plain " {
			require.ErrorContains(t, err, "rsa public key")
		} else {
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(v), got)
		}
	}
}
func TestR8bGitHandlersRejectInvalidSources(t *testing.T) {
	for _, mode := range []string{"missing", "archive", "builtin"} {
		t.Run(mode, func(t *testing.T) {
			rt := r8bRouter(t, &models.AISkill{})
			if mode != "missing" {
				s := &models.AISkill{Id: 1, Name: "demo", SourceType: "local"}
				if mode == "builtin" {
					s.SourceType = models.AISkillSourceGit
					s.CreatedBy = "system"
				}
				require.NoError(t, rt.Ctx.DB.Create(s).Error)
			}
			w := identityRequest(t, rt, rt.aiSkillGitInstallPut, map[string]interface{}{}, "1", &models.User{Username: "alice"})
			if mode == "missing" {
				require.Equal(t, 404, w.Code)
			} else {
				require.Equal(t, 400, w.Code)
			}
			if mode != "builtin" {
				w = identityRequest(t, rt, rt.aiSkillGitUpdate, map[string]interface{}{}, "1", &models.User{Username: "alice"})
				require.Contains(t, w.Body.String(), "skill")
			}
		})
	}
}
