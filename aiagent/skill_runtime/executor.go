package skillruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
	"github.com/toolkits/pkg/logger"
)

// Deps are the runtime collaborators the executor needs. They are primitives
// (not the aiagent ToolDeps) so this package stays free of an aiagent import.
type Deps struct {
	Sandbox    *sandbox.Sandbox
	DBCtx      *ctx.Context
	SkillsPath string
	Policy     sandbox.SkillPolicyConfig
}

// Request describes one skill execution. Identity (User) is bound by the caller
// from the chat session owner — the script never provides it (§12.1).
type Request struct {
	SkillName   string
	Entry       string   // optional entry override; "" → convention
	Args        []string // argv appended after the entry script
	Stdin       []byte
	User        *models.User
	SessionID   string // chat id
	TriggerType string // "llm_tool" / "api" / "test"
}

// Execute materializes the skill, builds the spec, runs it through the sandbox,
// audits it, and returns the fenced tool_result text. A *sandbox.DisabledError
// means execution is off on this host; a setup error (skill missing, no entry)
// is returned as-is. A script that runs and exits non-zero / times out is a
// success here — the outcome is encoded in the fenced output, not an error.
func Execute(c context.Context, d Deps, req Request) (string, error) {
	if d.Sandbox == nil || !d.Sandbox.Enabled() {
		reason := "sandbox not initialized"
		if d.Sandbox != nil {
			reason = d.Sandbox.DisabledReason()
		}
		return "", &sandbox.DisabledError{Reason: reason}
	}
	if err := validateSkillName(req.SkillName); err != nil {
		return "", err
	}

	skillDir := filepath.Join(d.SkillsPath, req.SkillName)
	if st, err := os.Stat(skillDir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("skill %q not found on disk", req.SkillName)
	}

	entry, err := resolveEntry(skillDir, req.Entry)
	if err != nil {
		return "", err
	}

	execID := newExecID()
	ws, err := d.Sandbox.NewWorkspace(execID)
	if err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	defer ws.Cleanup()

	if err := stageSkillFiles(skillDir, ws.Skill); err != nil {
		return "", fmt.Errorf("stage skill files: %w", err)
	}
	hostEntry := filepath.Join(ws.Skill, entry.Rel)

	ident := identity{SessionID: req.SessionID}
	if req.User != nil {
		ident.UserID = fmt.Sprintf("%d", req.User.Id)
		ident.Username = req.User.Username
	}
	trigger := req.TriggerType
	if trigger == "" {
		trigger = "llm_tool"
	}

	// Start this run's control channels (egress proxy + Skill Gateway, §10/§12)
	// and tear them down when it ends. On setup failure, degrade to a safe run
	// with no network and no gateway rather than launching half-wired.
	netMode := resolveNetwork(d.Sandbox)
	cc, cerr := setupControlChannels(d, execID, req.SkillName, netMode, req.User)
	if cerr != nil {
		logger.Warningf("sandbox: control channels for exec %s failed, running with network=none and no gateway: %v", execID, cerr)
		netMode = sandbox.NetworkNone
		cc = nil
	}
	defer cc.close()

	spec := buildExecSpec(d.Sandbox.Config(), ws, entry, hostEntry, req.Args, req.Stdin, ident, execID, req.SkillName, trigger, netMode, cc.mounts(), cc.env())

	res, runErr := d.Sandbox.Run(c, spec)
	d.writeAudit(req, spec, entry, res, runErr)
	if runErr != nil {
		return "", runErr
	}

	return FenceOutput(string(res.Stdout), string(res.Stderr), FenceMeta{
		SkillName: req.SkillName,
		ExitCode:  res.ExitCode,
		Note:      fenceNote(res),
	}), nil
}

func fenceNote(res sandbox.ExecResult) string {
	var notes []string
	if res.Timeout {
		notes = append(notes, "execution killed by timeout")
	} else if res.KilledBy != "" {
		notes = append(notes, "killed by "+res.KilledBy)
	}
	if res.StdoutTruncated || res.StderrTruncated {
		notes = append(notes, "output truncated at byte cap")
	}
	return strings.Join(notes, "; ")
}

func (d Deps) writeAudit(req Request, spec sandbox.ExecSpec, entry entryInfo, res sandbox.ExecResult, runErr error) {
	if d.DBCtx == nil {
		return
	}
	rec := &models.SandboxExecutionRecord{
		ExecId:          spec.ExecID,
		SessionId:       req.SessionID,
		SkillName:       req.SkillName,
		Entrypoint:      entry.Rel,
		Argv:            truncate(strings.Join(spec.Command, " "), 2000),
		Engine:          res.Engine,
		NetworkPolicy:   string(spec.Network),
		TriggerType:     spec.TriggerType,
		ExitCode:        res.ExitCode,
		Timeout:         res.Timeout,
		KilledBy:        res.KilledBy,
		DurationMs:      res.Duration.Milliseconds(),
		StdoutSample:    sample(res.Stdout, 2048),
		StderrSample:    sample(res.Stderr, 2048),
		StdoutTruncated: res.StdoutTruncated,
		StderrTruncated: res.StderrTruncated,
		CreatedAt:       time.Now().Unix(),
	}
	if req.User != nil {
		rec.UserId = req.User.Id
		rec.Username = req.User.Username
	}
	if runErr != nil {
		rec.ErrorMsg = truncate(runErr.Error(), 2000)
	} else if res.Error != "" {
		rec.ErrorMsg = truncate(res.Error, 2000)
	}
	if err := rec.Add(d.DBCtx); err != nil {
		logger.Warningf("sandbox: write audit record for exec %s failed: %v", spec.ExecID, err)
	}
}

func newExecID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "se_" + hex.EncodeToString(b[:])
}

func sample(b []byte, max int) string {
	if len(b) > max {
		return string(b[:max])
	}
	return string(b)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
