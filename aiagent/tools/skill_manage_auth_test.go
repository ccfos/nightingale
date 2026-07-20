package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

func adminUser() *models.User    { return &models.User{RolesLst: []string{models.AdminRole}} }
func standardUser() *models.User { return &models.User{RolesLst: []string{"Standard"}} }

// 创建技能的可见范围解析：非管理员恒私有；管理员以表单提交值为准（表单是用户刚做的
// 选择，比模型草拟参数时的 private 新）；都没有时默认私有，绝不默认成公共。
func TestResolveSkillPrivate(t *testing.T) {
	publicForm := map[string]string{aiagent.SkillScopeFieldKey: "1"}  // 全员可见
	privateForm := map[string]string{aiagent.SkillScopeFieldKey: "2"} // 仅管理团队可见
	publicArgs := map[string]interface{}{"private": float64(0)}

	cases := []struct {
		name   string
		user   *models.User
		args   map[string]interface{}
		params map[string]string
		want   int
	}{
		{"非管理员即便选了公共也强制私有", standardUser(), publicArgs, publicForm, 1},
		{"缺省私有", adminUser(), nil, nil, 1},
		{"管理员显式传 private=0", adminUser(), publicArgs, nil, 0},
		{"管理员表单选全员可见", adminUser(), nil, publicForm, 0},
		{"表单选择优先于模型参数", adminUser(), publicArgs, privateForm, 1},
		{"无法识别的表单值回落到参数", adminUser(), publicArgs, map[string]string{aiagent.SkillScopeFieldKey: "x"}, 0},
	}
	for _, c := range cases {
		if got := resolveSkillPrivate(c.user, c.args, c.params); got != c.want {
			t.Errorf("%s: resolveSkillPrivate = %d, want %d", c.name, got, c.want)
		}
	}
}

// 管理团队既可由模型直接给出，也可由缺参门表单经 params 注入；两者都没有时返回空，
// 由调用方弹表单而不是替用户瞎选。
func TestResolveCreationTeamIDsForSkill(t *testing.T) {
	fromForm := map[string]string{"team_ids": "3,4"}

	if got := resolveCreationTeamIDs(getArgInt64Slice(map[string]interface{}{"user_group_ids": []interface{}{float64(1), float64(2)}}, "user_group_ids"), fromForm); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("tool args should win over form-injected team_ids, got %v", got)
	}
	if got := resolveCreationTeamIDs(nil, fromForm); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("form-injected team_ids should be used when args carry none, got %v", got)
	}
	if got := resolveCreationTeamIDs(nil, nil); len(got) != 0 {
		t.Fatalf("no teams anywhere should stay empty (so the gate pops the form), got %v", got)
	}
}

// user_group_ids 的取值形状随模型漂移：真数组、数字字符串数组、整个数组被引号包起来
// 都要能解析；非正数/垃圾值丢弃，避免把 0 当成团队 ID 写进授权列表。
func TestGetArgInt64Slice(t *testing.T) {
	cases := []struct {
		name string
		val  interface{}
		want []int64
	}{
		{"JSON 数组", []interface{}{float64(1), float64(2)}, []int64{1, 2}},
		{"数字字符串元素", []interface{}{"1", " 2 "}, []int64{1, 2}},
		{"字符串化的数组", "[1,2]", []int64{1, 2}},
		{"丢弃非正数与垃圾值", []interface{}{float64(0), float64(-1), "abc", float64(5)}, []int64{5}},
		{"缺失", nil, nil},
	}
	for _, c := range cases {
		got := getArgInt64Slice(map[string]interface{}{"user_group_ids": c.val}, "user_group_ids")
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

// 提案腿把解析后的授权写回 args，确认腿在下一轮 replay 这份 args（JSON 往返后）时，
// 即便表单注入的 params 已经不在，也必须还原出同样的团队与可见范围——否则用户点
// 「确认」会再弹一次表单，或者管理员选的「全员可见」被悄悄降级成私有。
func TestSkillAuthSurvivesProposalReplay(t *testing.T) {
	// 提案腿：团队与可见范围都来自表单注入的 params。
	formParams := map[string]string{"team_ids": "7,8", aiagent.SkillScopeFieldKey: "1"}
	args := map[string]interface{}{"name": "redis-triage"}
	args["user_group_ids"] = resolveCreationTeamIDs(getArgInt64Slice(args, "user_group_ids"), formParams)
	args["private"] = resolveSkillPrivate(adminUser(), args, formParams)

	// 确认腿：args 经 JSON 序列化后原样 replay，params 里已经没有表单值。
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal replay args: %v", err)
	}
	var replayed map[string]interface{}
	if err := json.Unmarshal(raw, &replayed); err != nil {
		t.Fatalf("unmarshal replay args: %v", err)
	}

	teams := resolveCreationTeamIDs(getArgInt64Slice(replayed, "user_group_ids"), nil)
	if len(teams) != 2 || teams[0] != 7 || teams[1] != 8 {
		t.Fatalf("teams lost on replay: %v", teams)
	}
	if got := resolveSkillPrivate(adminUser(), replayed, nil); got != 0 {
		t.Fatalf("admin scope lost on replay: private = %d, want 0", got)
	}
}

// 授权表单是 input 类中断：被中断的那次 create_skill 不进 transcript，表单交回来时
// 模型手里只剩对话文本，重写的正文/脚本必然漂移或截断。草稿必须让内容原样还原——
// 这是端到端的关键不变式：表单往返后 instructions 与 files 与首次提交逐字节一致。
func TestSkillDraftSurvivesAuthFormRoundTrip(t *testing.T) {
	ctx := context.Background()
	deps := &aiagent.ToolDeps{} // 无 Redis：走 proposalStore 的进程内兜底
	params := map[string]string{"chat_id": "chat-draft-1", "seq_id": "7"}
	// 表单提交那一轮：团队/可见范围经 action.param → params 直达工具层。
	resumed := map[string]string{"chat_id": "chat-draft-1", "seq_id": "8", "team_ids": "5", aiagent.SkillScopeFieldKey: "2"}

	instructions := strings.Repeat("你是一个 Redis 排查助手，按以下决策树排查。\n", 200)
	script := "#!/usr/bin/env python3\nprint('disk report')\n"
	first := map[string]interface{}{
		"name":           "redis-slowlog-triage",
		"description":    "排查 Redis 慢查询。用户说 Redis 慢、连接数打满时使用",
		"instructions":   instructions,
		"builtin_tools":  []interface{}{"list_datasources"},
		"max_iterations": float64(20),
		"files":          []interface{}{map[string]interface{}{"path": "main.py", "content": script}},
	}
	if err := stashSkillDraft(ctx, deps, params, first); err != nil {
		t.Fatalf("stash draft: %v", err)
	}

	// 表单提交后的那一轮：模型凭对话文本重建，正文被截断、脚本干脆丢了，技能名还多了
	// 个尾随空格——两侧都规范化后仍应认成同一个技能。
	rebuilt := map[string]interface{}{
		"name":         "redis-slowlog-triage ",
		"description":  "排查 Redis 慢查询",
		"instructions": "你是一个 Redis 排查助手。",
	}
	restored, halt := restoreSkillDraft(ctx, deps, resumed, rebuilt)
	if halt != "" {
		t.Fatalf("resume turn should restore the draft, got halt: %s", halt)
	}

	if got := getArgString(restored, "instructions"); got != instructions {
		t.Fatalf("instructions drifted across the form round trip: %d chars, want %d", len(got), len(instructions))
	}
	if got := getArgString(restored, "description"); got != first["description"] {
		t.Fatalf("description drifted: %q", got)
	}
	files, err := parseSkillFiles(restored)
	if err != nil {
		t.Fatalf("parse restored files: %v", err)
	}
	if len(files) != 1 || files[0].Name != "main.py" || files[0].Content != script {
		t.Fatalf("script file lost or altered across the form round trip: %+v", files)
	}

	// 提案生成后草稿被消费；此后再有恢复轮只能中止，绝不能拿模型重写的正文顶上。
	dropSkillDraft(ctx, deps, params)
	if _, halt := restoreSkillDraft(ctx, deps, resumed, rebuilt); halt == "" {
		t.Fatal("a consumed draft must abort the resume turn instead of falling back to the rebuilt args")
	}
}

// 草稿缺失/损坏/技能名对不上时必须中止：静默回退到模型这轮的参数，等于把没人审过的
// 重写稿送进提案。
func TestSkillDraftAbortsWhenUnusable(t *testing.T) {
	ctx := context.Background()
	deps := &aiagent.ToolDeps{}
	resumed := map[string]string{"chat_id": "chat-draft-abort", "seq_id": "2", "team_ids": "5"}
	rebuilt := map[string]interface{}{"name": "skill-a", "instructions": "重写的正文"}

	// 草稿从未存过（超时被清、或换了实例且没配 Redis）。
	if _, halt := restoreSkillDraft(ctx, deps, resumed, rebuilt); halt != skillDraftLostMsg {
		t.Fatalf("missing draft should abort with the lost-draft copy, got %q", halt)
	}

	// 技能名对不上：中止文案要点出草稿里的名字，方便模型改用它重试。
	if err := stashSkillDraft(ctx, deps, resumed, map[string]interface{}{"name": "skill-a", "instructions": "A 的正文"}); err != nil {
		t.Fatalf("stash draft: %v", err)
	}
	_, halt := restoreSkillDraft(ctx, deps, resumed, map[string]interface{}{"name": "skill-b", "instructions": "B 的正文"})
	if halt == "" || !contains(halt, "skill-a") {
		t.Fatalf("name mismatch should abort and name the drafted skill, got %q", halt)
	}
}

// 草稿只在授权表单的续跑轮生效：用户没走表单而是直接改口（「正文改成 X 再建」）时，
// 模型这轮的参数才是最新意图，草稿既不覆盖也不中止。
func TestSkillDraftOnlyAppliesOnFormResume(t *testing.T) {
	ctx := context.Background()
	deps := &aiagent.ToolDeps{}
	stashed := map[string]string{"chat_id": "chat-draft-2", "seq_id": "1"}
	if err := stashSkillDraft(ctx, deps, stashed, map[string]interface{}{"name": "skill-a", "instructions": "A 的旧正文"}); err != nil {
		t.Fatalf("stash draft: %v", err)
	}

	revised := map[string]interface{}{"name": "skill-a", "instructions": "用户要求改过的新正文"}
	got, halt := restoreSkillDraft(ctx, deps, stashed, revised) // 无 team_ids：不是表单续跑
	if halt != "" || getArgString(got, "instructions") != "用户要求改过的新正文" {
		t.Fatalf("a non-form turn must keep the model's args: halt=%q instructions=%q", halt, getArgString(got, "instructions"))
	}

	// 换个会话即便带着表单值也取不到本会话的草稿——只能中止，不能串会话还原。
	otherChat := map[string]string{"chat_id": "chat-draft-3", "team_ids": "5"}
	if _, halt := restoreSkillDraft(ctx, deps, otherChat, revised); halt != skillDraftLostMsg {
		t.Fatalf("drafts must not cross conversations, got halt %q", halt)
	}

	// 无 chat_id（CLI / A2A 无会话入口）：不启用草稿机制，原样放行。
	noChat := map[string]string{"team_ids": "5"}
	if got, halt := restoreSkillDraft(ctx, deps, noChat, revised); halt != "" || getArgString(got, "instructions") != "用户要求改过的新正文" {
		t.Fatalf("missing chat_id should be a no-op: halt=%q", halt)
	}
}

// 草稿是内容的唯一权威：草稿里没有的内容字段必须被清掉，否则还原结果仍取决于模型
// 这一轮多写了什么（例如凭空补出一个 files）。
func TestSkillDraftDropsFieldsAbsentFromDraft(t *testing.T) {
	ctx := context.Background()
	deps := &aiagent.ToolDeps{}
	params := map[string]string{"chat_id": "chat-draft-4"}
	resumed := map[string]string{"chat_id": "chat-draft-4", "team_ids": "5"}
	if err := stashSkillDraft(ctx, deps, params, map[string]interface{}{"name": "no-files", "instructions": "正文"}); err != nil {
		t.Fatalf("stash draft: %v", err)
	}

	restored, halt := restoreSkillDraft(ctx, deps, resumed, map[string]interface{}{
		"name":         "no-files",
		"instructions": "正文",
		"files":        []interface{}{map[string]interface{}{"path": "main.py", "content": "print(1)"}},
	})
	if halt != "" {
		t.Fatalf("resume turn should restore the draft, got halt: %s", halt)
	}
	files, err := parseSkillFiles(restored)
	if err != nil {
		t.Fatalf("parse restored files: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("hallucinated file should not survive the restore: %+v", files)
	}
}

// 确认文案必须把授权信息写全：用户点「确认」时看到的就是即将落库的团队与可见范围。
func TestRenderSkillProposalCarriesAuth(t *testing.T) {
	prompt := renderSkillProposal("即将创建技能", "redis-triage", "排查 Redis", "你是一个排查助手", nil, nil, 0, "管理团队 运维组（成员可编辑）；可见范围 仅管理团队可见")
	for _, want := range []string{"运维组", "仅管理团队可见", "正文：8 字"} {
		if !contains(prompt, want) {
			t.Fatalf("proposal copy missing %q: %s", want, prompt)
		}
	}
	// 修改技能不动授权，文案里就不该出现授权行，免得误导用户以为会改。
	if contains(renderSkillProposal("即将修改技能", "redis-triage", "排查 Redis", "你是一个排查助手", nil, nil, 0, ""), "授权") {
		t.Fatal("update proposal should not render an auth line")
	}
}

func TestSkillScopeText(t *testing.T) {
	if got := skillScopeText(0); got != "全员可见" {
		t.Fatalf("private=0 should render 全员可见, got %q", got)
	}
	if got := skillScopeText(1); got != "仅管理团队可见" {
		t.Fatalf("private=1 should render 仅管理团队可见, got %q", got)
	}
}
