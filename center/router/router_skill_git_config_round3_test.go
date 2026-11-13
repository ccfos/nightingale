package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/stretchr/testify/require"
)

func gitConfigString(value string) *string { return &value }

func TestGitConfigForInstall_EncryptsTokenForStorage(t *testing.T) {
	rt := &Router{}
	private, public, err := secu.GenerateRsaKeyPair("")
	require.NoError(t, err)
	rt.HTTP.RSA.RSAPublicKey = public
	rt.HTTP.RSA.RSAPrivateKey = private
	req := aiSkillGitRequest{
		GitURL: gitConfigString("https://example.invalid/skills.git"), GitRefType: gitConfigString(skill.GitRefBranch),
		GitRef: gitConfigString("main"), GitAuthType: gitConfigString(skill.GitAuthToken), GitToken: gitConfigString(" secret "),
	}

	cfg, stored, err := rt.gitConfigForInstall(req)

	require.NoError(t, err)
	require.Equal(t, "secret", cfg.Token)
	require.Contains(t, stored.Token, "enc:")
	require.NotContains(t, stored.Token, "secret")
	token, err := rt.gitTokenForUse(stored.Token)
	require.NoError(t, err)
	require.Equal(t, "secret", token)
	require.Equal(t, cfg.URL, stored.URL)
	require.Equal(t, cfg.Ref, stored.Ref)
}

func TestGitConfigForInstall_RequiresTokenAndEncryptionKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		token   *string
		message string
	}{
		{name: "missing token", message: "git_token is required"},
		{name: "missing key", token: gitConfigString("secret"), message: "rsa public key is not configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			req := aiSkillGitRequest{
				GitURL: gitConfigString("https://example.invalid/skills.git"), GitRefType: gitConfigString(skill.GitRefBranch),
				GitRef: gitConfigString("main"), GitAuthType: gitConfigString(skill.GitAuthToken), GitToken: tc.token,
			}

			_, _, err := rt.gitConfigForInstall(req)

			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestGitConfigForUpdate_BuiltinIgnoresSubmittedSourceAndToken(t *testing.T) {
	rt := &Router{}
	current := &models.AISkill{GitInfo: &models.AISkillGitInfo{
		URL: "https://example.invalid/original.git", RefType: skill.GitRefBranch, Ref: "main",
		AuthType: skill.GitAuthToken, Token: "stored-token", Subdir: "skills/original",
	}}
	req := aiSkillGitRequest{
		GitURL: gitConfigString("https://example.invalid/replacement.git"), GitRef: gitConfigString("other"),
		GitToken: gitConfigString("replacement-token"), GitSubdir: gitConfigString("other"),
	}

	cfg, stored, err := rt.gitConfigForUpdate(current, req, true)

	require.NoError(t, err)
	require.Equal(t, current.GitInfo.URL, cfg.URL)
	require.Equal(t, "main", cfg.Ref)
	require.Equal(t, "skills/original", cfg.Subdir)
	require.Equal(t, "stored-token", cfg.Token)
	require.Equal(t, "stored-token", stored.Token)
}

func TestAISkillGitInstallPut_UpdatesSourceAndScopeTogether(t *testing.T) {
	rt := skillCRUDRouter(t)
	current := seedCRUDSkill(t, rt)
	require.NoError(t, rt.Ctx.DB.Model(&current).Updates(map[string]interface{}{
		"source_type": models.AISkillSourceGit,
		"git_info":    `{"url":"https://example.invalid/old.git","ref_type":"branch","ref":"main","auth_type":"token","token":"old"}`,
	}).Error)
	private := 0
	req := aiSkillGitRequest{
		GitURL: gitConfigString("https://example.invalid/new.git"), GitRefType: gitConfigString(skill.GitRefTag),
		GitRef: gitConfigString("v1"), GitAuthType: gitConfigString(skill.GitAuthNone), GitSubdir: gitConfigString("skills/demo"),
		Private: &private, UserGroupIds: []int64{10},
	}

	w := identityRequest(t, rt, rt.aiSkillGitInstallPut, req, "1", &models.User{Id: 1, Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	saved, err := models.AISkillGetById(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "https://example.invalid/new.git", saved.GitInfo.URL)
	require.Equal(t, skill.GitRefTag, saved.GitInfo.RefType)
	require.Equal(t, "v1", saved.GitInfo.Ref)
	require.Equal(t, "skills/demo", saved.GitInfo.Subdir)
	require.Empty(t, saved.GitInfo.Token)
	require.Equal(t, 0, saved.Private)
	require.Equal(t, []int64{10}, saved.UserGroupIds)
	require.Equal(t, "alice", saved.UpdatedBy)
	require.Equal(t, current.Instructions, saved.Instructions)
}

func TestAISkillGitInstallPut_RejectsOutsiderWithoutChangingSource(t *testing.T) {
	rt := skillCRUDRouter(t)
	current := seedCRUDSkill(t, rt)
	require.NoError(t, rt.Ctx.DB.Model(&current).Update("source_type", models.AISkillSourceGit).Error)

	w := identityRequest(t, rt, rt.aiSkillGitInstallPut,
		aiSkillGitRequest{UserGroupIds: []int64{99}}, "1", &models.User{Id: 2, Username: "outsider"})

	require.Equal(t, http.StatusForbidden, w.Code)
	saved, err := models.AISkillGetById(rt.Ctx, 1)
	require.NoError(t, err)
	require.Nil(t, saved.GitInfo)
	require.Equal(t, current.UserGroupIds, saved.UserGroupIds)
	require.Equal(t, current.Private, saved.Private)
}
