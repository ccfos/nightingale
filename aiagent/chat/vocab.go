package chat

import "strings"

// creationSkillSpec declares, for one n9e-create-* skill, the keywords that
// identify it from the user's raw input and the context keys the skill
// requires before the agent can usefully run. The preflight for the
// "creation" action_key walks this table: first match by keyword, then halt
// the turn with a single form_select response covering every missing context
// key. The frontend renders one progressive form and submits all picks at
// once, avoiding a multi-round ping-pong.
//
// The table is append-only — add a new skill by appending a row. When
// multiple entries keyword-match the same input the first match wins, so put
// more specific skills before broader ones.
//
// requiredContexts ordering is meaningful: it dictates the order fields
// appear in the form, and the frontend locks later fields until earlier ones
// are picked (progressive disclosure).
type creationSkillSpec struct {
	skillName        string
	keywords         []string
	requiredContexts []string // supported keys: "busi_group_id" | "datasource_id" | "team_ids"
}

var creationSkills = []creationSkillSpec{
	// Alert subscribe / mute must come before the generic "告警" keyword used
	// by alert-rule, otherwise "创建订阅" would route to alert-rule.
	{"n9e-create-alert-subscribe", []string{"告警订阅", "订阅规则", "subscribe"}, []string{"busi_group_id"}},
	{"n9e-create-alert-mute", []string{"屏蔽", "静默", "mute"}, []string{"busi_group_id"}},
	{"n9e-create-notify-rule", []string{"通知规则", "notify rule", "notify"}, []string{"team_ids"}},
	// Dashboard 只需业务组——面板可以跨数据源，数据源交给 LLM 从 page context
	// 或 list_datasources 自行解决，preflight 强制选一个反而限制了后续灵活性。
	{"n9e-create-dashboard", []string{"仪表盘", "dashboard", "面板"}, []string{"busi_group_id"}},
	{"n9e-create-alert-rule", []string{"告警规则", "告警", "alert rule"}, []string{"busi_group_id", "datasource_id"}},
}

// matchCreationSkill returns the first creationSkillSpec whose keyword is
// contained in userInput. Match is case-insensitive.
// Returns nil when no keyword hits — callers treat this as "don't intervene,
// let the agent's own skill auto-selection handle it".
func matchCreationSkill(userInput string) *creationSkillSpec {
	lower := strings.ToLower(userInput)
	for i := range creationSkills {
		for _, kw := range creationSkills[i].keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return &creationSkills[i]
			}
		}
	}
	return nil
}

// creationVerbs and queryVerbs drive the intent fast-path. Kept conservative:
// false negatives are fine (we fall back to the LLM classifier), but false
// positives would mis-route queries like "查询已创建的告警规则" into creation.
var (
	creationVerbs = []string{"创建", "新建", "添加", "加一条", "加一个", "建一条", "建一个", "创一条", "创一个", "create ", "add ", "build ", "make "}
	// queryVerbs act as an anti-signal — if either a query verb or a past-tense
	// creation phrase is present, we refuse the fast-path even when other
	// signals align.
	queryVerbs = []string{"查看", "查询", "查一下", "查下", "看一下", "看下", "列出", "有哪些", "显示", "已创建", "已新建", "已添加", " show ", " list ", " get ", " view "}
)

// HasCreationIntent returns true when the user input unambiguously asks to
// create a new resource. Requires all three signals:
//  1. A creation verb ("创建", "新建", "create", ...).
//  2. A creationSkills keyword (告警规则, 仪表盘, 屏蔽, ...).
//  3. No query verb ("查看", "已创建", "list", ...) that would flip the intent.
//
// This is used as a routing fast-path in processAssistantMessage to bypass
// the LLM classifier (which has been timing out at 15s and falling back to
// general_chat — see WARNING.log "intent inference failed").
func HasCreationIntent(userInput string) bool {
	if !hasAnyKeyword(userInput, creationVerbs) {
		return false
	}
	if matchCreationSkill(userInput) == nil {
		return false
	}
	if hasAnyKeyword(userInput, queryVerbs) {
		return false
	}
	return true
}

func hasAnyKeyword(s string, kws []string) bool {
	lower := strings.ToLower(s)
	for _, kw := range kws {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
