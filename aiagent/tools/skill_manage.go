package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	skillpkg "github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

// =============================================================================
// Skill authoring tools — let the agent create/edit user skills mid-conversation
// (the engine behind the skill-creator skill).
//
// Persistence reuses the exact model layer the HTTP route uses (AISkill.Create /
// AISkillFileBatchUpsert), so both surfaces converge on one write path. The one
// deliberate difference: we synthesize the SKILL.md ourselves (via
// skill.BuildSkillMD with builtin_tools + max_iterations) and store it as the
// SKILL.md file row, so the DB→FS sync materializes those frontmatter-only
// fields verbatim — the plain JSON HTTP path can't carry them (no column).
//
// Writes are doubly guarded: the /ai-config/skills permission (same as the HTTP
// route) AND the shared two-phase confirmation gate (proposeUpdate /
// confirmUpdateGate) so the model drafts freely but nothing lands without an
// explicit user "确认".
// =============================================================================

// PermAISkills mirrors the rt.perm("/ai-config/skills") guard on the HTTP skill
// CRUD routes. Authoring a skill — especially a script skill that runs code in
// the sandbox — is a privileged operation, so we hold the AI surface to the same
// bar as the management UI.
const PermAISkills = "/ai-config/skills"

// proposalKindSkill namespaces skill proposals in the shared confirmation gate.
const proposalKindSkill = "ai_skill"

// proposalKindSkillDraft namespaces the pending-create draft stashed while the
// user fills in the authorization form. A separate kind from proposalKindSkill
// on purpose: confirmUpdateGate matches on kind, so a draft can never be passed
// off as an approved write proposal.
const proposalKindSkillDraft = "ai_skill_draft"

// skillContentArgs are the create_skill arguments that describe the skill itself
// (everything except its name and authorization). They are what the draft
// restores verbatim across the authorization-form round trip.
var skillContentArgs = []string{"description", "instructions", "builtin_tools", "files", "max_iterations", "compatibility"}

// authoringToolNames are excluded from list_skill_builtin_tools: a user skill
// has no business declaring the meta/authoring tools (it would loop or confuse
// the model). run_skill_script is also excluded — it's auto-injected when the
// sandbox is enabled, not something a skill declares.
var authoringToolNames = map[string]struct{}{
	"load_skill":               {},
	"run_skill_script":         {},
	"list_skill_builtin_tools": {},
	"get_skill":                {},
	"create_skill":             {},
	"update_skill":             {},
}

// skillNameRe constrains skill names to kebab-case: the name doubles as an
// on-disk directory and a catalog key, so we keep it to a portable, traversal-
// proof character set.
var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func init() {
	register(defs.ListSkillBuiltinTools, listSkillBuiltinTools)
	register(defs.GetSkill, getSkill)
	register(defs.CreateSkill, createSkill)
	register(defs.UpdateSkill, updateSkill)
}

// listSkillBuiltinTools returns the catalog of builtin tools a new skill may
// reference, so the author wires real tool names instead of hallucinated ones.
func listSkillBuiltinTools(_ context.Context, _ *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	search := strings.ToLower(strings.TrimSpace(getArgString(args, "search")))

	allDefs := aiagent.GetAllBuiltinToolDefs()
	type row struct{ name, desc string }
	rows := make([]row, 0, len(allDefs))
	for _, d := range allDefs {
		if _, skip := authoringToolNames[d.Name]; skip {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(d.Name), search) && !strings.Contains(strings.ToLower(d.Description), search) {
			continue
		}
		rows = append(rows, row{d.Name, d.Description})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	if len(rows) == 0 {
		return "没有匹配的内置工具。去掉 search 过滤再试，或确认工具名拼写。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("可在技能 builtin_tools 中引用的内置工具（共 %d 个）：\n\n", len(rows)))
	for _, r := range rows {
		desc := r.desc
		if n := []rune(desc); len(n) > 100 {
			desc = string(n[:100]) + "…"
		}
		sb.WriteString(fmt.Sprintf("- `%s` — %s\n", r.name, desc))
	}
	return sb.String(), nil
}

// getSkill returns a skill's stored SKILL.md (with frontmatter) plus its file
// list, so the author can read the current definition before editing.
func getSkill(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAISkills); err != nil {
		return "", err
	}

	name := strings.TrimSpace(getArgString(args, "name"))
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	// 私有 skill 对未授权用户不可读：与 load_skill 共用同一份隐藏名单（含 fail-closed
	// 的 deny-all），命中即当作不存在，避免 AI 工具面成为读私有内容的绕过口。
	if deps.IsSkillHidden(name) {
		return fmt.Sprintf("技能 %q 不存在。用 create_skill 新建，或确认技能名拼写。", name), nil
	}

	obj, err := models.AISkillGetByName(deps.DBCtx, name)
	if err != nil {
		return "", fmt.Errorf("failed to load skill: %v", err)
	}
	if obj == nil {
		return fmt.Sprintf("技能 %q 不存在。用 create_skill 新建，或确认技能名拼写。", name), nil
	}

	files, err := models.AISkillFileGetContents(deps.DBCtx, obj.Id)
	if err != nil {
		return "", fmt.Errorf("failed to load skill files: %v", err)
	}

	var skillMD string
	var others []string
	for _, f := range files {
		if f.Name == "SKILL.md" {
			skillMD = f.Content
			continue
		}
		others = append(others, fmt.Sprintf("- %s (%d bytes)", f.Name, f.Size))
	}
	sort.Strings(others)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 技能 %s\n\n", obj.Name))
	if obj.CreatedBy == "system" {
		sb.WriteString("> 这是**内置技能**，不可通过 update_skill 修改。\n\n")
	}
	sb.WriteString(fmt.Sprintf("- enabled: %v\n- created_by: %s\n\n", obj.Enabled, obj.CreatedBy))
	if skillMD != "" {
		sb.WriteString("## SKILL.md\n\n```markdown\n")
		sb.WriteString(skillMD)
		sb.WriteString("\n```\n")
	} else {
		sb.WriteString("## instructions\n\n")
		sb.WriteString(obj.Instructions)
		sb.WriteString("\n")
	}
	if len(others) > 0 {
		sb.WriteString("\n## 附带文件\n\n")
		sb.WriteString(strings.Join(others, "\n"))
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// createSkill persists a brand-new user skill (two-phase confirmed).
func createSkill(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAISkills); err != nil {
		return "", err
	}

	// 授权表单弹过一次的话，技能内容以服务端草稿为准（见 restoreSkillDraft）。
	// 确认腿的 args 由运行时原样重放、已是全量，不需要也不应该再被草稿覆盖。
	if !getArgBool(args, "confirmed") {
		restored, halt := restoreSkillDraft(ctx, deps, params, args)
		if halt != "" {
			return halt, nil
		}
		args = restored
	}

	name := strings.TrimSpace(getArgString(args, "name"))
	if err := validateSkillName(name); err != nil {
		return "", err
	}
	description := strings.TrimSpace(getArgString(args, "description"))
	if description == "" {
		return "", fmt.Errorf("description is required: 写清楚「用户说什么话/什么场景该用这个技能」，这是技能能否被自动选用的关键")
	}
	instructions := strings.TrimSpace(getArgString(args, "instructions"))
	if instructions == "" {
		return "", fmt.Errorf("instructions is required: 技能正文（操作/排查流程），不要写 frontmatter")
	}

	builtinTools, err := validatedBuiltinTools(args)
	if err != nil {
		return "", err
	}
	files, err := parseSkillFiles(args)
	if err != nil {
		return "", err
	}
	maxIter := getArgInt(args, "max_iterations", 0)
	compatibility := strings.TrimSpace(getArgString(args, "compatibility"))

	// Pre-gate collision check so we never propose a create that can't land.
	if exist, err := models.AISkillGetByName(deps.DBCtx, name); err != nil {
		return "", fmt.Errorf("failed to check existing skill: %v", err)
	} else if exist != nil {
		return "", fmt.Errorf("技能 %q 已存在；如需修改请用 update_skill", name)
	}

	// 授权：管理团队必填（成员据此获得编辑权），可见范围仅管理员可选——与「AI 配置 →
	// 技能」页面的创建表单同一口径。团队缺失时弹表单让用户选，而不是替用户挑一个。
	private := resolveSkillPrivate(user, args, params)
	userGroupIds := resolveSkillTeamIDs(getArgInt64Slice(args, "user_group_ids"), params)
	if len(userGroupIds) == 0 {
		// 不属于任何团队的非管理员不弹表单：候选团队为空的表单永远无法提交（前端确认
		// 按钮要求至少选一个团队），子集校验（resolveSkillTeamNames）也注定全部拒绝。
		// 直接把原因和出路说清楚，不进入草稿与表单流程。
		if !user.IsAdmin() {
			mine, err := getUserGroupIds(deps, user.Id)
			if err != nil {
				return "", err
			}
			if len(mine) == 0 {
				return aiagent.LangText(params["lang"],
					"你当前不属于任何团队，而技能必须授权给管理团队，因此暂时无法创建技能。请先让管理员把你加入某个团队，再来创建。",
					"You don't belong to any team yet, but a skill must be assigned managing teams, so it can't be created for now. Ask an administrator to add you to a team first, then try again."), nil
			}
		}
		// 草稿存不下就不能弹表单：表单一弹，这次的正文/脚本就随中断丢了。无会话上下文
		// （CLI / A2A）时同理——没有可靠的草稿键，宁可让模型把团队直接写进参数里，
		// 也不能弹一个注定丢正文的表单。
		if err := stashSkillDraft(ctx, deps, params, args); err != nil {
			logger.Warningf("create_skill: stash draft in chat %q failed: %v", params["chat_id"], err)
			return "", fmt.Errorf("无法暂存技能草稿（%v），因此没有弹出团队选择表单——弹了这次写好的正文/脚本就会丢失。"+
				"请改为直接在 create_skill 里带上 user_group_ids（可先用 list_teams 查团队 ID），或稍后重试", err)
		}
		return "", skillAuthFormInterrupt(params["lang"], deps, user, private)
	}
	teamNames, err := resolveSkillTeamNames(deps, user, userGroupIds)
	if err != nil {
		return "", err
	}

	skillMD, err := skillpkg.BuildSkillMD(&skillpkg.DBSkill{
		Name:          name,
		Description:   description,
		Compatibility: compatibility,
		BuiltinTools:  builtinTools,
		MaxIterations: maxIter,
		Instructions:  instructions,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build SKILL.md: %v", err)
	}

	// Two-phase confirmation. Propose leg ends the turn with a deterministic
	// approval prompt; the confirm leg is replayed by the runtime with
	// confirmed=true (zero LLM involvement) — see update_proposal.go.
	if !getArgBool(args, "confirmed") {
		// 把解析出的授权写回 args：确认腿是下一轮用这份 args 原样 replay 的，而表单注入的
		// params（team_ids/skill_scope）只存在于表单提交那一轮，届时已经取不到——不写回
		// 就会在用户点「确认」时再弹一次表单，或把管理员选的「全员可见」悄悄降级成私有。
		args["user_group_ids"] = userGroupIds
		args["private"] = private
		prompt := renderSkillProposal("即将创建技能", name, description, instructions, builtinTools, files, maxIter,
			fmt.Sprintf("管理团队 %s（成员可编辑）；可见范围 %s", strings.Join(teamNames, "、"), skillScopeText(private)))
		out, err := proposeUpdate(ctx, deps, params, &updateProposal{
			Kind:         proposalKindSkill,
			TargetID:     0,
			BaselineHash: "",
			Changes:      []string{"create skill " + name},
		}, prompt, args)
		// 提案成功（返回的是确认中断）后草稿才使命完成——ResumeArgs 已带全量参数；
		// 留着它会在用户改主意、重新建一个同名技能时灌回旧正文。提案没生成出来则
		// 保留草稿，让用户重试时仍能拿回原文。
		if _, proposed := err.(*aiagent.ToolInterrupt); proposed {
			dropSkillDraft(ctx, deps, params)
		}
		return out, err
	}
	if _, err := confirmUpdateGate(ctx, deps, params, "create_skill", proposalKindSkill, 0, getArgString(args, "proposal_id"), true, ""); err != nil {
		return "", err
	}

	fileRows := append([]*models.AISkillFile{
		{Name: "SKILL.md", Content: skillMD, CreatedBy: user.Username},
	}, files...)

	obj := models.AISkill{
		Name:          name,
		Description:   description,
		Instructions:  instructions,
		Compatibility: compatibility,
		Enabled:       true,
		UserGroupIds:  userGroupIds,
		Private:       private,
		CreatedBy:     user.Username,
		UpdatedBy:     user.Username,
	}
	if err := obj.Verify(); err != nil {
		return "", err
	}

	err = models.DB(deps.DBCtx).Transaction(func(tx *gorm.DB) error {
		tCtx := txContext(deps.DBCtx, tx)
		if err := obj.Create(tCtx); err != nil {
			return err
		}
		return models.AISkillFileBatchUpsert(tCtx, obj.Id, fileRows, false)
	})
	if err != nil {
		return "", fmt.Errorf("failed to create skill: %v", err)
	}

	materializeSkillToDisk(deps, obj.Id, name)
	logger.Infof("create_skill: user=%s name=%s id=%d tools=%d files=%d teams=%v private=%d",
		user.Username, name, obj.Id, len(builtinTools), len(files), userGroupIds, private)

	return renderSkillResult("已创建技能", name, obj.Enabled, len(files), deps), nil
}

// updateSkill patches an existing user skill (two-phase confirmed). Only the
// provided fields change; the rest are read back from the skill's current
// SKILL.md frontmatter so a partial edit doesn't drop tool bindings.
func updateSkill(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAISkills); err != nil {
		return "", err
	}

	name := strings.TrimSpace(getArgString(args, "name"))
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	obj, err := models.AISkillGetByName(deps.DBCtx, name)
	if err != nil {
		return "", fmt.Errorf("failed to load skill: %v", err)
	}
	if obj == nil {
		return fmt.Sprintf("技能 %q 不存在；如需新建请用 create_skill。", name), nil
	}
	if obj.CreatedBy == "system" {
		return fmt.Sprintf("技能 %q 是内置技能，不可修改。", name), nil
	}
	// AI 工具面与 HTTP 路由共用同一套编辑授权，避免成为绕过入口：私有 skill 对未授权
	// 用户当作不存在；可见但不在授权团队的（如他人团队的公共 skill）也拒绝修改。
	if deps.IsSkillHidden(name) {
		return fmt.Sprintf("技能 %q 不存在；如需新建请用 create_skill。", name), nil
	}
	gids, err := getUserGroupIds(deps, user.Id)
	if err != nil {
		return "", fmt.Errorf("failed to load user groups: %v", err)
	}
	if !obj.CanBeEditedBy(user, gids) {
		return fmt.Sprintf("技能 %q 无权修改：仅授权团队成员、创建人或更新人可编辑。", name), nil
	}

	// Read current frontmatter so unspecified fields keep their value. The stored
	// SKILL.md file is the source of truth for builtin_tools / max_iterations
	// (no DB column holds them).
	curMD, curMeta, curBody, err := loadSkillMarkdown(deps, obj.Id)
	if err != nil {
		return "", err
	}

	description := obj.Description
	if v := strings.TrimSpace(getArgString(args, "description")); v != "" {
		description = v
	}
	instructions := curBody
	if v := strings.TrimSpace(getArgString(args, "instructions")); v != "" {
		instructions = v
	}
	if strings.TrimSpace(instructions) == "" {
		instructions = obj.Instructions
	}
	compatibility := obj.Compatibility
	if v := strings.TrimSpace(getArgString(args, "compatibility")); v != "" {
		compatibility = v
	}

	builtinTools := curMeta.BuiltinTools
	if _, present := args["builtin_tools"]; present {
		bt, err := validatedBuiltinTools(args)
		if err != nil {
			return "", err
		}
		builtinTools = bt
	}

	maxIter := curMeta.MaxIterations
	if v := getArgInt(args, "max_iterations", -1); v >= 0 {
		maxIter = v
	}

	files, err := parseSkillFiles(args)
	if err != nil {
		return "", err
	}

	skillMD, err := skillpkg.BuildSkillMD(&skillpkg.DBSkill{
		Name:          obj.Name,
		Description:   description,
		License:       obj.License,
		Compatibility: compatibility,
		Metadata:      obj.Metadata,
		AllowedTools:  obj.AllowedTools,
		BuiltinTools:  builtinTools,
		MaxIterations: maxIter,
		Instructions:  instructions,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build SKILL.md: %v", err)
	}

	// Conflict guard baseline: the current stored SKILL.md. If it changes between
	// propose and confirm (someone else edited the skill), the gate rejects.
	baseline := hashConfigs(curMD)

	if !getArgBool(args, "confirmed") {
		prompt := renderSkillProposal("即将修改技能", obj.Name, description, instructions, builtinTools, files, maxIter, "")
		return proposeUpdate(ctx, deps, params, &updateProposal{
			Kind:         proposalKindSkill,
			TargetID:     obj.Id,
			BaselineHash: baseline,
			Changes:      []string{"update skill " + obj.Name},
		}, prompt, args)
	}
	if _, err := confirmUpdateGate(ctx, deps, params, "update_skill", proposalKindSkill, obj.Id, getArgString(args, "proposal_id"), true, baseline); err != nil {
		return "", err
	}

	fileRows := append([]*models.AISkillFile{
		{Name: "SKILL.md", Content: skillMD, CreatedBy: user.Username},
	}, files...)

	ref := *obj
	ref.Description = description
	ref.Instructions = instructions
	ref.Compatibility = compatibility
	ref.UpdatedBy = user.Username

	err = models.DB(deps.DBCtx).Transaction(func(tx *gorm.DB) error {
		tCtx := txContext(deps.DBCtx, tx)
		if err := obj.Update(tCtx, ref); err != nil {
			return err
		}
		// fullSync=false: upsert SKILL.md + the files the user passed, leave any
		// other attached files untouched.
		return models.AISkillFileBatchUpsert(tCtx, obj.Id, fileRows, false)
	})
	if err != nil {
		return "", fmt.Errorf("failed to update skill: %v", err)
	}

	materializeSkillToDisk(deps, obj.Id, obj.Name)
	logger.Infof("update_skill: user=%s name=%s id=%d tools=%d files=%d", user.Username, obj.Name, obj.Id, len(builtinTools), len(files))

	return renderSkillResult("已更新技能", obj.Name, ref.Enabled, len(files), deps), nil
}

// =============================================================================
// helpers
// =============================================================================

// validateSkillName enforces kebab-case and rejects names colliding with a
// builtin skill (a masked DB skill would be dead weight — see AISkillSync).
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 100 {
		return fmt.Errorf("技能名过长（最多 100 字符）")
	}
	if !skillNameRe.MatchString(name) {
		return fmt.Errorf("技能名 %q 不合法：只能用小写字母、数字和连字符（kebab-case），如 redis-slowlog-triage", name)
	}
	if skillpkg.IsBuiltinName(name) {
		return fmt.Errorf("技能名 %q 与内置技能冲突，请换一个名字", name)
	}
	return nil
}

// validatedBuiltinTools parses the builtin_tools arg and rejects any name that
// isn't a registered builtin tool — so a created skill never carries a binding
// to a tool that doesn't exist.
func validatedBuiltinTools(args map[string]interface{}) ([]string, error) {
	names := argStringSlice(args, "builtin_tools")
	if len(names) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	var unknown []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		if _, ok := aiagent.GetBuiltinToolDef(n); !ok {
			unknown = append(unknown, n)
			continue
		}
		out = append(out, n)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("以下 builtin_tools 不存在：%s。用 list_skill_builtin_tools 查可用工具名后再试", strings.Join(unknown, ", "))
	}
	return out, nil
}

// parseSkillFiles coerces the files arg (array of {path|name, content}) into
// AISkillFile rows, validating relative paths. SKILL.md is rejected — it's
// synthesized from the structured fields, not supplied as a file, so the two
// can't drift apart.
func parseSkillFiles(args map[string]interface{}) ([]*models.AISkillFile, error) {
	raw, ok := args["files"]
	if !ok || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("files 必须是数组，每项为对象 {path, content}")
	}
	out := make([]*models.AISkillFile, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for i, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("files[%d] 必须是对象 {path, content}", i)
		}
		path := strings.TrimSpace(asString(m["path"]))
		if path == "" {
			path = strings.TrimSpace(asString(m["name"]))
		}
		if path == "" {
			return nil, fmt.Errorf("files[%d] 缺少 path", i)
		}
		rel, err := safeSkillFilePath(path)
		if err != nil {
			return nil, fmt.Errorf("files[%d] 路径 %q 非法：%v", i, path, err)
		}
		if rel == "SKILL.md" {
			return nil, fmt.Errorf("不要把 SKILL.md 作为 file 传入：技能正文写进 instructions 参数即可，SKILL.md 由工具自动生成")
		}
		if _, dup := seen[rel]; dup {
			return nil, fmt.Errorf("files 中存在重复路径 %q", rel)
		}
		seen[rel] = struct{}{}
		out = append(out, &models.AISkillFile{Name: rel, Content: asString(m["content"])})
	}
	return out, nil
}

// safeSkillFilePath normalizes a user-supplied file path and rejects absolute
// paths / `..` escapes / separators that would break out of the skill dir. Same
// rules the DB→FS sync enforces (skill.safeRelPath), re-checked at ingest.
func safeSkillFilePath(name string) (string, error) {
	n := strings.TrimSpace(strings.ReplaceAll(name, `\`, "/"))
	if n == "" {
		return "", fmt.Errorf("empty")
	}
	if strings.HasPrefix(n, "/") {
		return "", fmt.Errorf("不允许绝对路径")
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径不能包含 ..")
		}
		if seg == "." {
			return "", fmt.Errorf("路径段不能为 .")
		}
	}
	return n, nil
}

// loadSkillMarkdown returns the skill's stored SKILL.md raw content, its parsed
// frontmatter, and its body. Used by update to merge partial edits onto the
// current definition.
func loadSkillMarkdown(deps *aiagent.ToolDeps, skillId int64) (raw string, meta skillpkg.Frontmatter, body string, err error) {
	files, err := models.AISkillFileGetContents(deps.DBCtx, skillId)
	if err != nil {
		return "", skillpkg.Frontmatter{}, "", fmt.Errorf("failed to load skill files: %v", err)
	}
	for _, f := range files {
		if f.Name == "SKILL.md" {
			raw = f.Content
			break
		}
	}
	if raw == "" {
		return "", skillpkg.Frontmatter{}, "", nil
	}
	if fm, instructions, ok := skillpkg.ParseMarkdown(raw); ok {
		return raw, fm, instructions, nil
	}
	return raw, skillpkg.Frontmatter{}, raw, nil
}

// materializeSkillToDisk writes the just-saved skill to the on-disk registry so
// it's loadable on the next turn instead of waiting up to one DB→FS sync
// interval. Best-effort: the periodic sync loop is the backstop, so a failure
// here is logged, not surfaced as a tool error.
func materializeSkillToDisk(deps *aiagent.ToolDeps, skillId int64, name string) {
	if deps.SkillsPath == "" {
		logger.Warningf("skill %q saved but SkillsPath unset; relying on periodic sync to materialize", name)
		return
	}
	files, err := models.AISkillFileGetContents(deps.DBCtx, skillId)
	if err != nil {
		logger.Warningf("skill %q: load files for materialize failed (periodic sync will backstop): %v", name, err)
		return
	}
	dbFiles := make([]skillpkg.DBSkillFile, 0, len(files))
	for _, f := range files {
		dbFiles = append(dbFiles, skillpkg.DBSkillFile{Name: f.Name, Content: f.Content})
	}
	if err := skillpkg.SyncOneDBSkill(deps.SkillsPath, skillpkg.DBSkill{Name: name, Files: dbFiles}); err != nil {
		logger.Warningf("skill %q: materialize to disk failed (periodic sync will backstop): %v", name, err)
	}
}

// txContext rebuilds a tx-scoped *ctx.Context from the request context, mirroring
// the router's transaction pattern so model writes share one atomic unit.
func txContext(base *ctx.Context, tx *gorm.DB) *ctx.Context {
	return &ctx.Context{DB: tx, CenterApi: base.CenterApi, Ctx: base.Ctx, IsCenter: base.IsCenter}
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// renderSkillProposal builds the deterministic confirmation copy shown to the
// user before a create/update lands — the tool renders it, never the model, so
// what the user approves is exactly what will be written. auth 为授权说明（管理
// 团队 + 可见范围），仅创建时有值：修改不动授权，写出来反而误导。
// 正文与附带文件带上字数：只列文件名的话，正文/脚本内容被换掉用户也看不出来。
func renderSkillProposal(action, name, description, instructions string, builtinTools []string, files []*models.AISkillFile, maxIter int, auth string) string {
	kind := "知识/流程型"
	if hasScriptFile(files) {
		kind = "脚本型（含可执行脚本）"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s **%s**：\n", action, name))
	sb.WriteString(fmt.Sprintf("\n- 类型：%s", kind))
	sb.WriteString(fmt.Sprintf("\n- 触发描述：%s", truncateRunes(description, 200)))
	if instructions != "" {
		sb.WriteString(fmt.Sprintf("\n- 正文：%d 字，开头「%s」", len([]rune(instructions)), truncateRunes(strings.TrimSpace(instructions), 60)))
	}
	if len(builtinTools) > 0 {
		sb.WriteString(fmt.Sprintf("\n- 绑定工具：%s", strings.Join(builtinTools, ", ")))
	}
	if maxIter > 0 {
		sb.WriteString(fmt.Sprintf("\n- 单轮工具上限：%d", maxIter))
	}
	if len(files) > 0 {
		names := make([]string, 0, len(files))
		for _, f := range files {
			names = append(names, fmt.Sprintf("%s（%d 字）", f.Name, len([]rune(f.Content))))
		}
		sb.WriteString(fmt.Sprintf("\n- 附带文件：%s", strings.Join(names, ", ")))
	}
	if auth != "" {
		sb.WriteString(fmt.Sprintf("\n- 授权：%s", auth))
	}
	sb.WriteString("\n\n以上尚未写入。回复「确认」立即生效，回复「取消」放弃，也可以直接提出调整。")
	return sb.String()
}

// =============================================================================
// 创建技能的待授权草稿
//
// 授权表单是 input 类中断，而 input 中断不做确定性重放（router 的 tryResumePending
// 直接带着补全的 Context 重跑 agent），被中断的那次调用也不进 transcript（native.go
// 会把 assistant 轮的 tool_calls 裁到已执行的部分）。也就是说表单交回来时，模型手里
// 只剩对话文本，instructions / files 原文已经不在上下文里——让它凭记忆重写，长正文
// 必然漂移甚至被历史窗口截断，而提案文案只列描述和文件名，用户根本看不出内容变了样。
//
// 所以首次调用的完整参数按会话暂存在服务端（复用 update_proposal 的 Redis/内存双写
// 存储，多实例部署同样有效），表单回来后确定性还原：本次调用只负责技能名与授权。
// =============================================================================

// skillDraftID 按会话给草稿定 key：一个会话同时只可能有一个待授权的创建，且这个 id
// 必须能在恢复轮由服务端自行推出——被中断的调用没有 tool result，模型无从携带它。
func skillDraftID(chatID string) string { return "skill_draft:" + chatID }

// skillDraftLostMsg 是草稿在恢复轮拿不到时的中止文案。这里绝不能回退到模型这轮的
// 参数：那份正文是凭对话重写的，可能已经漂移/截断，而用户在提案里只会看到字数和开头，
// 未必察觉。宁可中止让用户重新起草确认，也不落一份没人审过的正文。
const skillDraftLostMsg = "上一步暂存的技能内容（正文/附带脚本）已失效（超时或服务重启）。" +
	"选完团队后我手里的正文是凭对话重写的，可能与你确认过的原稿不一致，" +
	"所以本次创建已中止——请重新完整起草技能内容、展示给用户确认后，再调用 create_skill。"

// restoreSkillDraft 用暂存草稿覆盖本次调用的内容字段，返回 (参数, 中止文案)。
// 中止文案非空时调用方必须把它当作工具结果直接回给模型：不写库、也不进入提案。
//
// 是否为「授权表单的续跑」看本轮有没有带技能授权表单自己的提交值（skill_team_ids 经
// action.param 直达工具层）。这个键是技能专用的：与通知规则共用 team_ids 的话，用户在
// 同一会话里提交通知规则的团队表单、本轮又顺带创建技能，就会被误判成续跑并因查无草稿
// 而中止一次本来合法的创建。用户没走表单而是直接改口（「正文改成 X 再建」）时同理——
// 模型这轮的参数才是用户的最新意图，草稿不参与（残留草稿会在下次弹表单时被新草稿覆盖）。
// 续跑轮里草稿缺失/损坏/技能名对不上一律中止，不做静默回退。技能名两侧都去空白后比较：
// 模型两轮分别写出 "x " 和 "x" 不该被当成两个技能。
func restoreSkillDraft(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string, args map[string]interface{}) (map[string]interface{}, string) {
	if strings.TrimSpace(params[aiagent.SkillTeamsFieldKey]) == "" {
		return args, ""
	}
	chatID := params["chat_id"]
	if chatID == "" {
		// 表单只可能由有会话的入口弹出（没有 chat_id 时 stashSkillDraft 会拒绝弹表单），
		// 走到这里说明提交值来路不明，按草稿丢失处理，不拿模型重写的正文顶上。
		return args, skillDraftLostMsg
	}
	p := updateProposals.peek(ctx, deps.Redis, skillDraftID(chatID))
	if p == nil || p.Kind != proposalKindSkillDraft {
		logger.Warningf("create_skill: no draft for chat %s on the auth-form resume turn; aborting instead of trusting the rebuilt args", chatID)
		return args, skillDraftLostMsg
	}
	var draft map[string]interface{}
	if err := json.Unmarshal([]byte(p.Payload), &draft); err != nil {
		logger.Warningf("create_skill: corrupt draft in chat %s: %v", chatID, err)
		return args, skillDraftLostMsg
	}
	drafted := strings.TrimSpace(getArgString(draft, "name"))
	if cur := strings.TrimSpace(getArgString(args, "name")); drafted != cur {
		return args, fmt.Sprintf("暂存草稿对应的技能是 %q，本次却要创建 %q，内容对不上，已中止。"+
			"如果要建的就是 %q，用这个名字重新调用一次 create_skill 即可（正文无需重发）；"+
			"如果确实要建另一个技能，请重新完整起草内容并让用户确认。", drafted, cur, drafted)
	}

	merged := make(map[string]interface{}, len(args)+len(skillContentArgs))
	for k, v := range args {
		merged[k] = v
	}
	// 草稿是内容的唯一权威：草稿没有的内容字段要删掉，而不是留下模型这轮补出来的值，
	// 否则还原结果仍取决于模型这次写了什么。
	for _, k := range skillContentArgs {
		if v, ok := draft[k]; ok {
			merged[k] = v
		} else {
			delete(merged, k)
		}
	}
	logger.Infof("create_skill: restored draft for skill %q in chat %s", drafted, chatID)
	return merged, ""
}

// stashSkillDraft 在弹授权表单前暂存本次调用的完整参数。存不下必须让调用方放弃弹表单：
// 表单一弹，这次调用的参数就随中断被丢掉了（native.go 不把被中断的调用写进 transcript），
// 没有草稿兜底就只能靠模型重写正文——那正是本机制要消灭的漂移。
// 无 chat_id（CLI / A2A 等无会话入口）时没有可靠的草稿键，同样按失败处理：让调用方改走
// 「参数里直接带 user_group_ids」的路径，那条路径不产生中断，正文不会丢。
func stashSkillDraft(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string, args map[string]interface{}) error {
	chatID := params["chat_id"]
	if chatID == "" {
		return fmt.Errorf("当前入口没有会话上下文（chat_id），无处暂存草稿")
	}
	body, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal draft: %v", err)
	}
	seqID, _ := strconv.ParseInt(params["seq_id"], 10, 64)
	return updateProposals.put(ctx, deps.Redis, &updateProposal{
		ID:        skillDraftID(chatID),
		ChatID:    chatID,
		SeqID:     seqID,
		Kind:      proposalKindSkillDraft,
		Changes:   []string{"draft skill " + strings.TrimSpace(getArgString(args, "name"))},
		Payload:   string(body),
		CreatedAt: time.Now(),
	})
}

// dropSkillDraft 消费掉草稿（take 即删）。
func dropSkillDraft(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string) {
	if chatID := params["chat_id"]; chatID != "" {
		updateProposals.take(ctx, deps.Redis, skillDraftID(chatID))
	}
}

// skillScopeText 把 Private 渲染成与技能页面一致的中文说法（确认文案不经 LLM 转述）。
func skillScopeText(private int) string {
	if private == 0 {
		return "全员可见"
	}
	return "仅管理团队可见"
}

// resolveSkillTeamNames 校验授权团队并返回其名称（用于确认文案）：团队必须真实存在；
// 非管理员只能授权给自己所属的团队——要求提交的团队全部属于自己（子集）而非仅有交集，
// 与 HTTP 创建路由 aiSkillAdd 同一口径，避免 AI 面成为把技能授权给无关团队的绕过口。
func resolveSkillTeamNames(deps *aiagent.ToolDeps, user *models.User, ids []int64) ([]string, error) {
	groups, err := models.UserGroupGetByIds(deps.DBCtx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to load teams: %v", err)
	}
	nameOf := make(map[int64]string, len(groups))
	for _, g := range groups {
		nameOf[g.Id] = g.Name
	}

	var mine []int64
	if !user.IsAdmin() {
		if mine, err = getUserGroupIds(deps, user.Id); err != nil {
			return nil, err
		}
	}

	names := make([]string, 0, len(ids))
	var unknown, forbidden []string
	for _, id := range ids {
		name, ok := nameOf[id]
		if !ok {
			unknown = append(unknown, fmt.Sprintf("%d", id))
			continue
		}
		if !user.IsAdmin() && !int64SliceContains(mine, id) {
			forbidden = append(forbidden, name)
			continue
		}
		names = append(names, name)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("管理团队 ID %s 不存在：用 list_teams 查真实团队再试", strings.Join(unknown, ", "))
	}
	if len(forbidden) > 0 {
		return nil, fmt.Errorf("无权把技能授权给团队 %s：你只能授权给自己所属的团队，请改选后重试", strings.Join(forbidden, "、"))
	}
	return names, nil
}

func renderSkillResult(action, name string, enabled bool, fileCount int, deps *aiagent.ToolDeps) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s **%s**（已启用：%v）。\n", action, name, enabled))
	sb.WriteString("\n你可以在「AI 配置 → 技能」里查看和管理它。")
	if hasSandbox(deps) {
		sb.WriteString("如果它是脚本型技能，可以让我用 run_skill_script 跑一遍验证。")
	}
	return sb.String()
}

func hasScriptFile(files []*models.AISkillFile) bool {
	for _, f := range files {
		l := strings.ToLower(f.Name)
		if strings.HasSuffix(l, ".py") || strings.HasSuffix(l, ".sh") {
			return true
		}
	}
	return false
}

func hasSandbox(deps *aiagent.ToolDeps) bool {
	return deps != nil && deps.Sandbox != nil && deps.Sandbox.Enabled()
}
