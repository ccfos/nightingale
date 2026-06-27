package skillruntime

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/sandbox"
)

// identity is the acting principal bound to an execution (§12.1). The script
// never supplies it; it comes from the chat session owner.
type identity struct {
	UserID    string
	Username  string
	SessionID string
}

// buildExecSpec synthesizes the OS-agnostic ExecSpec from the global policy
// envelope + convention-inferred runtime + bound identity (§11.2). Network is
// the caller-resolved netMode (Egress preset × engine caps — see
// resolveNetwork; default Egress=open → proxy on bubblewrap); resources start
// from the default policy and are clamped to the skill ceilings; env is a clean
// whitelist (host env never inherited, §9.4) plus the control-channel env
// (HTTP(S)_PROXY → forwarder, gateway socket). Command/Cwd use real host paths
// (the unsafe engine runs them directly); Mounts carry the canonical
// /skill,/input,/workspace,/output targets for the Linux engines to bind.
func buildExecSpec(
	cfg sandbox.Config,
	ws *sandbox.Workspace,
	entry entryInfo,
	hostEntry string,
	args []string,
	stdin []byte,
	ident identity,
	execID, skillName, trigger string,
	netMode sandbox.NetworkPolicy,
	controlMounts []sandbox.MountSpec,
	controlEnv map[string]string,
) sandbox.ExecSpec {
	res := cfg.DefaultResources()
	clampResources(&res, cfg.Skill)

	cmd := append([]string{entry.Interp, hostEntry}, args...)

	// Clean, non-inherited environment with the Python essentials (§9.4).
	// PATH must be set explicitly: engines that --clearenv (bubblewrap) give the
	// child NO environment, so without it the interpreter ("python3"/"bash")
	// won't resolve inside the sandbox. The list covers the rootfs interpreter
	// locations (python:3-slim ships python3 under /usr/local/bin).
	env := map[string]string{
		"PATH":                    "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG":                    "C.UTF-8",
		"LC_ALL":                  "C.UTF-8",
		"HOME":                    ws.Workspace,
		"TMPDIR":                  ws.Workspace,
		"PYTHONDONTWRITEBYTECODE": "1",
		"PYTHONUNBUFFERED":        "1",
	}
	// Control-channel env (HTTP(S)_PROXY → forwarder, gateway socket path). Added
	// last; it never overrides the essentials above (disjoint keys).
	for k, v := range controlEnv {
		env[k] = v
	}

	profile := "bash-minimal"
	if entry.Type == "python" {
		profile = "python-minimal"
	}

	return sandbox.ExecSpec{
		ExecID:    execID,
		UserID:    ident.UserID,
		SessionID: ident.SessionID,
		SkillName: skillName,
		Command:   cmd,
		Cwd:       ws.Workspace,
		Env:       env,
		Stdin:     stdin,
		Mounts: []sandbox.MountSpec{
			{Source: ws.Skill, Target: "/skill", ReadOnly: true},
			{Source: ws.Input, Target: "/input", ReadOnly: true},
			{Source: ws.Workspace, Target: "/workspace"},
			{Source: ws.Output, Target: "/output"},
		},
		ControlMounts: controlMounts,
		Resources:     res,
		Network:       netMode,
		Policy:        sandbox.SecurityProfile{Profile: profile, NoNewPrivs: true},
		TriggerType:   trigger,
		Audit:         map[string]string{"username": ident.Username},
	}
}

// clampResources lowers requested resources to the skill ceilings (§11.2 clamp).
func clampResources(res *sandbox.ResourceSpec, lim sandbox.SkillLimits) {
	if lim.MaxTimeoutSeconds > 0 {
		max := time.Duration(lim.MaxTimeoutSeconds) * time.Second
		if res.Timeout > max {
			res.Timeout = max
		}
	}
	if lim.MaxMemoryMB > 0 && res.MemoryMB > lim.MaxMemoryMB {
		res.MemoryMB = lim.MaxMemoryMB
	}
	if lim.MaxPids > 0 && res.Pids > lim.MaxPids {
		res.Pids = lim.MaxPids
	}
}
