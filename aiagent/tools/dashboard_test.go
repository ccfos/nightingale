package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newDashboardTestDeps builds a ToolDeps backed by an in-memory sqlite DB with
// the tables get_dashboard_detail touches (user / board / board_payload) and an
// admin user (id=1) so the perm + busi-group checks short-circuit.
func newDashboardTestDeps(t *testing.T) *aiagent.ToolDeps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Board{}, &models.BoardPayload{}, &models.BusiGroup{}))
	c := ctx.NewContext(context.Background(), db, true)
	require.NoError(t, models.DB(c).Create(&models.User{Id: 1, Username: "root", Roles: models.AdminRole}).Error)
	// busi group id=1 — the write handlers (update_dashboard) resolve the board's
	// owning group and check rw on it; admin short-circuits the rw check but the
	// group row must exist.
	require.NoError(t, models.DB(c).Create(&models.BusiGroup{Id: 1, Name: "default"}).Error)
	return &aiagent.ToolDeps{DBCtx: c}
}

// TestGetDashboardDetail_IncludeConfigLoadsPayload is the regression for the bug
// where include_config=true read board.Configs — which BoardGetByID never
// hydrates (it's a gorm:"-" field stored in the separate board_payload table) —
// so variables/panels/lint came back empty for every real dashboard.
func TestGetDashboardDetail_IncludeConfigLoadsPayload(t *testing.T) {
	deps := newDashboardTestDeps(t)

	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 7, GroupId: 1, Name: "demo"}).Error)
	const payload = `{
		"var":[{"name":"prom","type":"datasource","definition":"prometheus"},
		       {"name":"ident","type":"query","definition":"label_values(up, ident)","datasource":{"cate":"prometheus","value":"${prom}"}}],
		"panels":[{"id":"p1","type":"timeseries","name":"CPU","options":{"standardOptions":{"util":"percent"}},
		           "targets":[{"refId":"A","expr":"up{ident=~\"$missing\"}"}]}]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 7, payload))

	out, err := getDashboardDetail(context.Background(), deps,
		map[string]interface{}{"id": float64(7), "include_config": true},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	var resp struct {
		Variables    []variableSummary `json:"variables"`
		Panels       []panelSummary    `json:"panels"`
		VariableLint []string          `json:"variable_lint"`
		ConfigError  string            `json:"config_error"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))

	require.Empty(t, resp.ConfigError, "payload should parse cleanly")
	require.Len(t, resp.Variables, 2, "variables must be loaded from board_payload")
	require.Len(t, resp.Panels, 1, "panels must be loaded from board_payload")
	if resp.Panels[0].Unit != "percent" {
		t.Fatalf("panel unit = %q, want percent", resp.Panels[0].Unit)
	}
	// $missing is undefined → lint must surface it, proving lint ran on the
	// hydrated payload rather than an empty config.
	require.NotEmpty(t, resp.VariableLint, "lint should flag the undefined $missing ref")
}

// TestUpdateDashboard_PersistsPatchedPayload is the handler-level regression for
// the two-phase write path. The first (propose) call computes the change set but
// must NOT write — it returns applied=false plus a proposal_id, and the payload
// in board_payload is untouched. A second (confirm) call in a LATER chat turn,
// carrying that proposal_id + confirmed=true, is what actually persists the
// patched config. A board with a payload must NOT report "has no config payload
// to modify".
func TestUpdateDashboard_PersistsPatchedPayload(t *testing.T) {
	deps := newDashboardTestDeps(t)

	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 11, GroupId: 1, Name: "rw"}).Error)
	const payload = `{
		"var":[{"name":"ident","type":"query","definition":"label_values(up, ident)","label":"主机"}],
		"panels":[{"id":"p1","type":"timeseries","name":"CPU"}]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 11, payload))

	// Phase 1 (propose): compute the change set without writing. seq_id=1 marks
	// the turn the proposal was made in; the later confirm must use a larger one.
	out, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{
			"id":        float64(11),
			"variables": `[{"name":"ident","label":"实例"}]`,
		},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "1"})
	require.NoError(t, err)

	var proposeResp struct {
		Changes    []string `json:"changes"`
		ProposalID string   `json:"proposal_id"`
		Applied    bool     `json:"applied"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &proposeResp))
	require.False(t, proposeResp.Applied, "the propose call must not write")
	require.NotEmpty(t, proposeResp.ProposalID, "the propose call must return a proposal_id")
	require.NotEmpty(t, proposeResp.Changes, "a label change should produce a change entry")

	// The propose phase must NOT have touched the stored payload yet.
	pre, err := models.BoardPayloadGet(deps.DBCtx, 11)
	require.NoError(t, err)
	var preCfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(pre), &preCfg))
	preV0 := preCfg["var"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "主机", preV0["label"], "propose must not persist the patch")

	// Phase 2 (confirm): a later turn (seq_id=2) carrying the proposal_id +
	// confirmed=true is what actually writes.
	out, err = updateDashboard(context.Background(), deps,
		map[string]interface{}{
			"id":          float64(11),
			"proposal_id": proposeResp.ProposalID,
			"confirmed":   true,
		},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "2"})
	require.NoError(t, err)

	var confirmResp struct {
		Changes []string `json:"changes"`
		Applied bool     `json:"applied"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &confirmResp))
	require.True(t, confirmResp.Applied, "the confirm call must write")
	require.NotEmpty(t, confirmResp.Changes, "confirm should echo the applied changes")

	// Only now must the patched payload be persisted (and re-loadable).
	saved, err := models.BoardPayloadGet(deps.DBCtx, 11)
	require.NoError(t, err)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(saved), &cfg))
	vars := cfg["var"].([]interface{})
	v0 := vars[0].(map[string]interface{})
	require.Equal(t, "实例", v0["label"], "variable label must be persisted to board_payload after confirm")
}

// TestUpdateDashboard_RejectedConfirmDoesNotBurnProposal is the regression for
// the take-before-validate bug: a confirm rejected by the cross-turn gate (the
// model confirming in the SAME turn it proposed, which the gate forbids) must
// NOT consume the proposal — otherwise the user's genuine confirm next turn
// fails with "not found" and the edit can never land. The gate exists precisely
// because the model misbehaves, so this recovery path must work.
func TestUpdateDashboard_RejectedConfirmDoesNotBurnProposal(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 21, GroupId: 1, Name: "rw"}).Error)
	const payload = `{"var":[{"name":"ident","type":"query","definition":"label_values(up, ident)","label":"主机"}]}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 21, payload))

	// Propose in turn seq=5.
	out, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(21), "variables": `[{"name":"ident","label":"实例"}]`},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "5"})
	require.NoError(t, err)
	var pr struct {
		ProposalID string `json:"proposal_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &pr))
	require.NotEmpty(t, pr.ProposalID)

	// Misbehaving same-turn confirm (seq=5) → rejected by the cross-turn gate.
	_, err = updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(21), "proposal_id": pr.ProposalID, "confirmed": true},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "5"})
	require.Error(t, err, "same-turn confirm must be rejected")

	// The genuine confirm next turn (seq=6) must still find the proposal and apply it.
	out, err = updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(21), "proposal_id": pr.ProposalID, "confirmed": true},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "6"})
	require.NoError(t, err, "proposal must survive the rejected same-turn confirm")
	var cr struct {
		Applied bool `json:"applied"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &cr))
	require.True(t, cr.Applied, "next-turn confirm must apply the surviving proposal")

	saved, err := models.BoardPayloadGet(deps.DBCtx, 21)
	require.NoError(t, err)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(saved), &cfg))
	v0 := cfg["var"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "实例", v0["label"], "the patch must land after the recovered confirm")

	// And the proposal is now consumed — a replay must fail.
	_, err = updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(21), "proposal_id": pr.ProposalID, "confirmed": true},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "7"})
	require.Error(t, err, "a confirmed proposal must be single-use (no replay)")
}

// TestUpdateDashboard_TolerantPanelScalars is the end-to-end regression: the LLM
// emits step as a float and instant/hide as strings inside the panels JSON; the
// handler must parse it (not bail with "invalid panels JSON") and produce a
// proposal.
func TestUpdateDashboard_TolerantPanelScalars(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 33, GroupId: 1, Name: "rw"}).Error)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 33,
		`{"panels":[{"id":"p1","type":"timeseries","name":"CPU","datasourceCate":"prometheus","targets":[{"refId":"A","expr":"old"}]}]}`))

	out, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{
			"id":     float64(33),
			"panels": `[{"id":"p1","delete":"false","queries":[{"ref":"A","promql":"new","step":15.0,"instant":"false","hide":"true"}]}]`,
		},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "1"})
	require.NoError(t, err, "float/string scalars in panels JSON must not abort the parse")

	var pr struct {
		Changes []string `json:"changes"`
		Applied bool     `json:"applied"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &pr))
	require.False(t, pr.Applied)
	require.NotEmpty(t, pr.Changes)
}

// TestUpdateDashboard_ConfirmFailsClosedWithoutChatContext locks the fail-closed
// turn gate: a confirm that can't prove a later-turn human confirm (no chat_id /
// seq_id, e.g. a headless workflow) must be refused, leaving the board untouched.
func TestUpdateDashboard_ConfirmFailsClosedWithoutChatContext(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 31, GroupId: 1, Name: "rw"}).Error)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 31, `{"var":[{"name":"ident","type":"query","label":"主机"}]}`))

	// Propose with NO chat context.
	out, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(31), "variables": `[{"name":"ident","label":"实例"}]`},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)
	var pr struct {
		ProposalID string `json:"proposal_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &pr))

	// Confirm without chat/turn identity → must fail closed.
	_, err = updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(31), "proposal_id": pr.ProposalID, "confirmed": true},
		map[string]string{"user_id": "1"})
	require.Error(t, err, "a confirm without chat/turn identity must be refused")

	saved, err := models.BoardPayloadGet(deps.DBCtx, 31)
	require.NoError(t, err)
	require.Contains(t, saved, "主机")
	require.NotContains(t, saved, "实例", "the board must be untouched after a refused confirm")
}

// TestUpdateDashboard_ConfirmRejectsUnparseableSeq covers the other fail-open
// hole: chat_id matches but seq_id isn't numeric, so a later turn can't be
// proven — the gate must reject rather than skip the check.
func TestUpdateDashboard_ConfirmRejectsUnparseableSeq(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 32, GroupId: 1, Name: "rw"}).Error)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 32, `{"var":[{"name":"ident","type":"query","label":"主机"}]}`))

	out, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(32), "variables": `[{"name":"ident","label":"实例"}]`},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "1"})
	require.NoError(t, err)
	var pr struct {
		ProposalID string `json:"proposal_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &pr))

	_, err = updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(32), "proposal_id": pr.ProposalID, "confirmed": true},
		map[string]string{"user_id": "1", "chat_id": "c1", "seq_id": "not-a-number"})
	require.Error(t, err, "an unparseable seq_id must fail the turn gate closed")
}

// TestUpdateDashboard_RejectsFixDatasourceOnNonProm confirms the non-Prometheus
// guard fires at the handler level: fix_datasource on a MySQL board must error
// rather than corrupting its datasource config.
func TestUpdateDashboard_RejectsFixDatasourceOnNonProm(t *testing.T) {
	deps := newDashboardTestDeps(t)

	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 12, GroupId: 1, Name: "mysql-board"}).Error)
	const payload = `{
		"var":[{"name":"ds","type":"datasource","definition":"mysql"}],
		"panels":[{"id":"p1","type":"table","name":"rows","datasourceCate":"mysql"}]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 12, payload))

	_, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(12), "fix_datasource": true},
		map[string]string{"user_id": "1"})
	require.Error(t, err, "fix_datasource on a MySQL board must be rejected")
}

// TestGetDashboardDetail_IncludeConfigStringBool is the regression for the
// raw-.(bool) bug: the LLM sometimes emits include_config as the string "true"
// rather than a JSON bool. getArgBool must still turn the config summary on, or
// the edit flow's "before" snapshot comes back empty and the agent proposes blind.
func TestGetDashboardDetail_IncludeConfigStringBool(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 9, GroupId: 1, Name: "demo"}).Error)
	const payload = `{
		"var":[{"name":"ident","type":"query","definition":"label_values(up, ident)"}],
		"panels":[{"id":"p1","type":"timeseries","name":"CPU"}]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 9, payload))

	out, err := getDashboardDetail(context.Background(), deps,
		map[string]interface{}{"id": float64(9), "include_config": "true"}, // string, not bool
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	if _, ok := resp["variables"]; !ok {
		t.Fatalf("string-form include_config=\"true\" must still return the config summary: %s", out)
	}
}

// TestUpdateDashboard_FixDatasourceStringBool is the matching regression for the
// fix_datasource flag: a string "true" must trigger the repair (produce a
// proposal) instead of silently no-op'ing into "nothing to change".
func TestUpdateDashboard_FixDatasourceStringBool(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 13, GroupId: 1, Name: "prom-board"}).Error)
	// Prometheus board with a panel pinned to a literal datasource id — exactly
	// what fix_datasource repairs (repoint to the dashboard's datasource var).
	const payload = `{
		"var":[{"name":"prom","type":"datasource","definition":"prometheus"}],
		"panels":[{"id":"p1","type":"timeseries","name":"CPU","datasourceCate":"prometheus","datasourceValue":1}]
	}`
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 13, payload))

	out, err := updateDashboard(context.Background(), deps,
		map[string]interface{}{"id": float64(13), "fix_datasource": "true"}, // string, not bool
		map[string]string{"user_id": "1"})
	require.NoError(t, err, "string-form fix_datasource=\"true\" must not error as nothing-to-change")

	var resp struct {
		Changes []string `json:"changes"`
		Applied bool     `json:"applied"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	require.False(t, resp.Applied, "propose phase must not write")
	require.NotEmpty(t, resp.Changes, "fix_datasource via string bool must produce a change")
}

// TestGetDashboardDetail_WithoutIncludeConfig confirms the lean default path
// still returns only metadata (no payload query, no variables/panels).
func TestGetDashboardDetail_WithoutIncludeConfig(t *testing.T) {
	deps := newDashboardTestDeps(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 8, GroupId: 1, Name: "meta-only"}).Error)

	out, err := getDashboardDetail(context.Background(), deps,
		map[string]interface{}{"id": float64(8)},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &resp))
	if _, ok := resp["variables"]; ok {
		t.Fatalf("default path must not include variables: %s", out)
	}
	if resp["name"] != "meta-only" {
		t.Fatalf("name = %v, want meta-only", resp["name"])
	}
}
