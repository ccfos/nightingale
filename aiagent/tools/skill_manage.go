package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

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
		prompt := renderSkillProposal("即将创建技能", name, description, builtinTools, files, maxIter)
		return proposeUpdate(ctx, deps, params, &updateProposal{
			Kind:         proposalKindSkill,
			TargetID:     0,
			BaselineHash: "",
			Changes:      []string{"create skill " + name},
		}, prompt, args)
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
	logger.Infof("create_skill: user=%s name=%s id=%d tools=%d files=%d", user.Username, name, obj.Id, len(builtinTools), len(files))

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
		prompt := renderSkillProposal("即将修改技能", obj.Name, description, builtinTools, files, maxIter)
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
// what the user approves is exactly what will be written.
func renderSkillProposal(action, name, description string, builtinTools []string, files []*models.AISkillFile, maxIter int) string {
	kind := "知识/流程型"
	if hasScriptFile(files) {
		kind = "脚本型（含可执行脚本）"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s **%s**：\n", action, name))
	sb.WriteString(fmt.Sprintf("\n- 类型：%s", kind))
	sb.WriteString(fmt.Sprintf("\n- 触发描述：%s", truncateRunes(description, 200)))
	if len(builtinTools) > 0 {
		sb.WriteString(fmt.Sprintf("\n- 绑定工具：%s", strings.Join(builtinTools, ", ")))
	}
	if maxIter > 0 {
		sb.WriteString(fmt.Sprintf("\n- 单轮工具上限：%d", maxIter))
	}
	if len(files) > 0 {
		names := make([]string, 0, len(files))
		for _, f := range files {
			names = append(names, f.Name)
		}
		sb.WriteString(fmt.Sprintf("\n- 附带文件：%s", strings.Join(names, ", ")))
	}
	sb.WriteString("\n\n以上尚未写入。回复「确认」立即生效，回复「取消」放弃，也可以直接提出调整。")
	return sb.String()
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
