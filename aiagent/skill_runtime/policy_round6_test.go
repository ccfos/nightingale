package skillruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
	"github.com/stretchr/testify/require"
)

func TestBuildExecSpec_BindsIdentityResourcesAndPythonEnvironment(t *testing.T) {
	ws := &sandbox.Workspace{
		Skill: "/run/skill", Input: "/run/input", Workspace: "/run/workspace", Output: "/run/output",
	}
	cfg := sandbox.Config{
		DefaultPolicy: sandbox.PolicyConfig{TimeoutSeconds: 5, MemoryMB: 32, Pids: 8},
		Skill:         sandbox.SkillLimits{MaxTimeoutSeconds: 2, MaxMemoryMB: 16, MaxPids: 3},
	}
	mounts := []sandbox.MountSpec{{Source: "/host/gw.sock", Target: sandbox.GatewaySocketTarget}}

	spec := buildExecSpec(cfg, ws, entryInfo{Rel: "main.py", Interp: "python3", Type: "python"},
		"/run/skill/main.py", []string{"--host", "web 1"}, []byte("payload"),
		identity{UserID: "7", Username: "alice", SessionID: "chat-1"}, "exec-1", "triage", "api",
		sandbox.NetworkNone, mounts, map[string]string{sandbox.GatewaySocketEnv: sandbox.GatewaySocketTarget})

	require.Equal(t, []string{"python3", "/run/skill/main.py", "--host", "web 1"}, spec.Command)
	require.Equal(t, "/run/workspace", spec.Cwd)
	require.Equal(t, []byte("payload"), spec.Stdin)
	require.Equal(t, "7", spec.UserID)
	require.Equal(t, "chat-1", spec.SessionID)
	require.Equal(t, "exec-1", spec.ExecID)
	require.Equal(t, "triage", spec.SkillName)
	require.Equal(t, "api", spec.TriggerType)
	require.Equal(t, map[string]string{"username": "alice"}, spec.Audit)
	require.Equal(t, sandbox.NetworkNone, spec.Network)
	require.Equal(t, sandbox.SecurityProfile{Profile: "python-minimal", NoNewPrivs: true}, spec.Policy)
	require.Equal(t, 2*time.Second, spec.Resources.Timeout)
	require.Equal(t, int64(16), spec.Resources.MemoryMB)
	require.Equal(t, int64(3), spec.Resources.Pids)
	require.Equal(t, mounts, spec.ControlMounts)
	require.Equal(t, []sandbox.MountSpec{
		{Source: ws.Skill, Target: "/skill", ReadOnly: true},
		{Source: ws.Input, Target: "/input", ReadOnly: true},
		{Source: ws.Workspace, Target: "/workspace"},
		{Source: ws.Output, Target: "/output"},
	}, spec.Mounts)
	require.Equal(t, map[string]string{
		"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "HOME": ws.Workspace, "TMPDIR": ws.Workspace,
		"PYTHONDONTWRITEBYTECODE": "1", "PYTHONUNBUFFERED": "1",
		sandbox.GatewaySocketEnv: sandbox.GatewaySocketTarget,
	}, spec.Env)
}

func TestClampResources_PreservesValuesBelowCeilings(t *testing.T) {
	res := sandbox.ResourceSpec{Timeout: time.Second, MemoryMB: 8, Pids: 2, StdoutMax: 1024}
	want := res

	clampResources(&res, sandbox.SkillLimits{MaxTimeoutSeconds: 2, MaxMemoryMB: 16, MaxPids: 3})

	require.Equal(t, want, res)
}

func TestFenceNote_ExplainsTerminationAndTruncation(t *testing.T) {
	cases := []struct {
		name   string
		result sandbox.ExecResult
		want   string
	}{
		{name: "timeout takes precedence", result: sandbox.ExecResult{Timeout: true, KilledBy: "signal"},
			want: "execution killed by timeout"},
		{name: "signal", result: sandbox.ExecResult{KilledBy: "memory limit"}, want: "killed by memory limit"},
		{name: "stdout", result: sandbox.ExecResult{StdoutTruncated: true}, want: "output truncated at byte cap"},
		{name: "stderr", result: sandbox.ExecResult{StderrTruncated: true}, want: "output truncated at byte cap"},
		{
			name: "combined", result: sandbox.ExecResult{KilledBy: "signal", StdoutTruncated: true, StderrTruncated: true},
			want: "killed by signal; output truncated at byte cap",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			note := fenceNote(tc.result)

			require.Equal(t, tc.want, note)
		})
	}
}

func TestFenceOutput_BoundsUntrustedSkillLabel(t *testing.T) {
	label := "triage]\n" + strings.Repeat("x", 200)

	out := FenceOutput("ok", "", FenceMeta{SkillName: label})

	header := strings.SplitN(out, "\n", 2)[0]
	require.Contains(t, header, "skill="+"triage) "+strings.Repeat("x", 120)+" exit=0")
	require.NotContains(t, header, "skill=triage]")
	require.Contains(t, out, "--- stdout ---\nok\n[END UNTRUSTED SKILL OUTPUT")
}

func TestStageSkillFiles_CopiesNestedResourcesWithoutDatabaseMarker(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(source, skill.FromDBMarker), "")
	mustWrite(t, filepath.Join(source, "SKILL.md"), "instructions")
	mustWrite(t, filepath.Join(source, "references", "guide.txt"), "runbook")
	mustWrite(t, filepath.Join(dest, "references", "guide.txt"), "previous longer content")

	err := stageSkillFiles(source, dest)

	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dest, skill.FromDBMarker))
	for name, want := range map[string]string{"SKILL.md": "instructions", "references/guide.txt": "runbook"} {
		data, err := os.ReadFile(filepath.Join(dest, name))
		require.NoError(t, err)
		require.Equal(t, want, string(data))
	}
}

func TestStageSkillFiles_ReportsMissingSource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "missing")

	err := stageSkillFiles(source, t.TempDir())

	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestStageSkillFiles_ReportsDestinationConflict(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(source, "resources", "guide"), "runbook")
	mustWrite(t, filepath.Join(dest, "resources"), "occupied")

	err := stageSkillFiles(source, dest)

	require.Error(t, err)
	data, err := os.ReadFile(filepath.Join(dest, "resources"))
	require.NoError(t, err)
	require.Equal(t, "occupied", string(data))
}

func TestResolveEntry_SelectsSoleTopLevelScript(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "scripts", "helper.py"), "print(1)")
	mustWrite(t, filepath.Join(root, "collect.sh"), "echo ok")

	entry, err := resolveEntry(root, "")

	require.NoError(t, err)
	require.Equal(t, entryInfo{Rel: "collect.sh", Interp: "bash", Type: "bash"}, entry)
}

func TestResolveEntry_RejectsInvalidExplicitEntry(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "script.js"), "console.log(1)")
	cases := []struct {
		name  string
		entry string
		want  string
	}{
		{name: "missing", entry: "missing.sh", want: "not found in skill"},
		{name: "unsupported", entry: "script.js", want: "only .py and .sh"},
		{name: "absolute", entry: "/main.sh", want: "absolute entry not allowed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := resolveEntry(root, tc.entry)

			require.ErrorContains(t, err, tc.want)
			require.Equal(t, entryInfo{}, entry)
		})
	}
}

func TestResolveEntry_ReportsUnreadableSkillDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	_, err := resolveEntry(root, "")

	require.ErrorContains(t, err, "read skill dir:")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestExecute_RejectsMissingSkillBeforeCreatingWorkspace(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	sb := sandbox.New(sandbox.Config{Engine: "unsafe", DataDir: data})

	out, err := Execute(context.Background(), Deps{Sandbox: sb, SkillsPath: root}, Request{SkillName: "missing"})

	require.Empty(t, out)
	require.ErrorContains(t, err, `skill "missing" not found on disk`)
	entries, err := os.ReadDir(data)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestExecute_NonzeroScriptExitIsFencedResult(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "demo", "main.sh"), "printf 'problem' >&2; exit 7")
	sb := sandbox.New(sandbox.Config{Engine: "unsafe", DataDir: t.TempDir()})

	out, err := Execute(context.Background(), Deps{Sandbox: sb, SkillsPath: root}, Request{
		SkillName: "demo", TriggerType: "test", User: &models.User{Id: 7, Username: "alice"},
	})

	require.NoError(t, err)
	require.Contains(t, out, "skill=demo exit=7")
	require.Contains(t, out, "--- stderr ---\nproblem\n")
	require.True(t, strings.HasSuffix(out, "[END UNTRUSTED SKILL OUTPUT · nonce="+parseNonce(t, out)+"]"))
}

func TestSetupControlChannels_CleansDirectoryAfterGatewaySetupFailure(t *testing.T) {
	data := t.TempDir()
	sb := sandbox.New(sandbox.Config{Engine: "unsafe", DataDir: data})

	channels, err := setupControlChannels(Deps{
		Sandbox: sb, DBCtx: &ctxpkg.Context{}, N9eBaseURL: "http://127.0.0.1",
	}, "exec-failed", "demo", sandbox.NetworkNone, &models.User{})

	require.Nil(t, channels)
	require.ErrorContains(t, err, "start skill gateway:")
	require.ErrorContains(t, err, "no bound user id")
	require.NoDirExists(t, filepath.Join(data, "run", "exec-failed"))
}

func TestSetupControlChannels_ReportsControlDirectoryConflict(t *testing.T) {
	data := t.TempDir()
	sb := sandbox.New(sandbox.Config{Engine: "unsafe", DataDir: data})
	mustWrite(t, filepath.Join(data, "run"), "occupied")

	channels, err := setupControlChannels(Deps{
		Sandbox: sb, DBCtx: &ctxpkg.Context{}, N9eBaseURL: "http://127.0.0.1",
	}, "exec-failed", "demo", sandbox.NetworkNone, &models.User{})

	require.Nil(t, channels)
	require.ErrorContains(t, err, "create control dir:")
	dataBytes, err := os.ReadFile(filepath.Join(data, "run"))
	require.NoError(t, err)
	require.Equal(t, "occupied", string(dataBytes))
}

func TestExecute_RejectsInvalidSkillName(t *testing.T) {
	sb := sandbox.New(sandbox.Config{Engine: "unsafe", DataDir: t.TempDir()})

	out, err := Execute(context.Background(), Deps{Sandbox: sb, SkillsPath: t.TempDir()}, Request{SkillName: "../escape"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "illegal skill name")
}

func TestExecute_ReportsWorkspaceCreationFailure(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "demo", "main.sh"), "echo ok")
	sb := sandbox.New(sandbox.Config{Engine: "unsafe", DataDir: data})
	mustWrite(t, filepath.Join(data, "sessions"), "occupied")

	out, err := Execute(context.Background(), Deps{Sandbox: sb, SkillsPath: root}, Request{SkillName: "demo"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "create workspace:")
}

func TestCopyFile_RejectsDirectoryDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	mustWrite(t, source, "content")
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.Mkdir(dest, 0o755))

	err := copyFile(source, dest)

	require.Error(t, err)
	require.DirExists(t, dest)
}
