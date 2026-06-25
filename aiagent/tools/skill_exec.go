package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	skillruntime "github.com/ccfos/nightingale/v6/aiagent/skill_runtime"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
)

func init() {
	register(defs.RunSkillScript, runSkillScript)
}

// runSkillScript executes a skill's entry script inside the isolation sandbox
// and returns its output fenced as untrusted data (§13). Identity is bound from
// the chat session owner (params["user_id"]) — the model never supplies it, so
// it cannot impersonate another user (§12.1). When the sandbox is disabled on
// this host the tool returns a clean, actionable message (not a hard error) so
// the model can relay it to the user.
func runSkillScript(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	skillName := strings.TrimSpace(getArgString(args, "skill_name"))
	if skillName == "" {
		return "", fmt.Errorf("skill_name is required")
	}

	if deps.Sandbox == nil || !deps.Sandbox.Enabled() {
		reason := "skill 脚本执行未在本服务端启用"
		if deps.Sandbox != nil && deps.Sandbox.DisabledReason() != "" {
			reason = deps.Sandbox.DisabledReason()
		}
		return fmt.Sprintf("skill 脚本执行当前不可用：%s。请联系管理员在 sandbox 配置中启用。", reason), nil
	}

	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	out, err := skillruntime.Execute(ctx, skillruntime.Deps{
		Sandbox:    deps.Sandbox,
		DBCtx:      deps.DBCtx,
		SkillsPath: deps.SkillsPath,
		Policy:     deps.Sandbox.Config().SkillPolicy,
	}, skillruntime.Request{
		SkillName:   skillName,
		Entry:       strings.TrimSpace(getArgString(args, "entry")),
		Args:        argStringSlice(args, "args"),
		Stdin:       []byte(getArgString(args, "stdin")),
		User:        user,
		SessionID:   params["chat_id"],
		TriggerType: "llm_tool",
	})
	if err != nil {
		if sandbox.IsDisabled(err) {
			return fmt.Sprintf("skill 脚本执行不可用：%v", err), nil
		}
		return "", err
	}
	return out, nil
}

// argStringSlice coerces an arg into a []string, tolerating a JSON array, a Go
// []string, or a single/space-separated string (the LLM occasionally passes a
// scalar where an array is expected).
func argStringSlice(args map[string]interface{}, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, fmt.Sprintf("%v", e))
		}
		return out
	case []string:
		return t
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return strings.Fields(s)
	}
	return nil
}
