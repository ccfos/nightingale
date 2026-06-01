package tools

import (
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// parseAlertPack unmarshals an integration alert pack JSON (array of rules).
func parseAlertPack(t *testing.T, s string) []models.AlertRule {
	t.Helper()
	var rules []models.AlertRule
	if err := json.Unmarshal([]byte(s), &rules); err != nil {
		t.Fatalf("unmarshal alert pack: %v", err)
	}
	return rules
}

// A prometheus rule and a host (heartbeat) rule, as the Linux categraf pack
// ships them: disabled, datasource_ids:[0], legacy notify fields populated.
const alertPackFixture = `[
  {"name":"CPU high","cate":"prometheus","prod":"metric","disabled":1,
   "datasource_ids":[0],"notify_version":0,
   "notify_channels":["email"],"notify_groups":["1"],"callbacks":["http://x"],
   "rule_config":{"queries":[{"prom_ql":"cpu_usage_active > 80","severity":2}]}},
  {"name":"Target lost","cate":"host","prod":"host","disabled":1,
   "rule_config":{"queries":[{"key":"offset","op":">","values":[300]}],
                  "triggers":[{"type":"target_miss","severity":1,"duration":60}]}}
]`

// With a datasource id, the prometheus rule binds to it and the host rule does
// not. Both get enabled, regrouped, owner-stamped, and notify-reset.
func TestPrepareImportedAlertRule_BindsDatasource(t *testing.T) {
	rules := parseAlertPack(t, alertPackFixture)

	for i := range rules {
		prepareImportedAlertRule(&rules[i], 5, "alice", 7, 0, "", "")
	}

	prom, host := rules[0], rules[1]

	// Common transforms.
	for _, r := range rules {
		if r.GroupId != 5 {
			t.Fatalf("%s: group_id = %d, want 5", r.Name, r.GroupId)
		}
		if r.CreateBy != "alice" || r.UpdateBy != "alice" {
			t.Fatalf("%s: owner not stamped: create=%q update=%q", r.Name, r.CreateBy, r.UpdateBy)
		}
		if r.Disabled != 0 {
			t.Fatalf("%s: disabled = %d, want 0 (enabled)", r.Name, r.Disabled)
		}
		if r.Id != 0 || r.UUID != 0 {
			t.Fatalf("%s: identity not reset: id=%d uuid=%d", r.Name, r.Id, r.UUID)
		}
		if r.NotifyVersion != 1 {
			t.Fatalf("%s: notify_version = %d, want 1", r.Name, r.NotifyVersion)
		}
		if len(r.NotifyChannelsJSON) != 0 || len(r.NotifyGroupsJSON) != 0 || len(r.CallbacksJSON) != 0 {
			t.Fatalf("%s: legacy notify fields not cleared: %#v", r.Name, r)
		}
	}

	// Prometheus rule pinned to ds 7.
	if len(prom.DatasourceQueries) != 1 {
		t.Fatalf("prom datasource_queries = %d, want 1", len(prom.DatasourceQueries))
	}
	q := prom.DatasourceQueries[0]
	if q.Op != "in" || len(q.Values) != 1 {
		t.Fatalf("prom datasource_query = %#v, want in:[7]", q)
	}
	if len(prom.DatasourceIdsJson) != 1 || prom.DatasourceIdsJson[0] != 7 {
		t.Fatalf("prom datasource_ids = %v, want [7]", prom.DatasourceIdsJson)
	}

	// Host rule untouched on the datasource axis.
	if len(host.DatasourceQueries) != 0 {
		t.Fatalf("host rule should have no datasource binding, got %#v", host.DatasourceQueries)
	}
	if len(host.DatasourceIdsJson) != 0 {
		t.Fatalf("host rule should have no datasource_ids, got %v", host.DatasourceIdsJson)
	}
}

// Without a datasource id, a non-host rule whose template carries no binding
// falls back to "match all datasources"; host stays empty.
func TestPrepareImportedAlertRule_NoDatasourceFallsBackToAll(t *testing.T) {
	rules := parseAlertPack(t, alertPackFixture)
	for i := range rules {
		prepareImportedAlertRule(&rules[i], 5, "bob", 0, 0, "", "")
	}

	prom, host := rules[0], rules[1]
	if len(prom.DatasourceQueries) != 1 || prom.DatasourceQueries[0].Op != models.DataSourceQueryAll.Op {
		t.Fatalf("prom rule should fall back to DataSourceQueryAll, got %#v", prom.DatasourceQueries)
	}
	if len(host.DatasourceQueries) != 0 {
		t.Fatalf("host rule should have no datasource binding, got %#v", host.DatasourceQueries)
	}
}

// The preview summary digs the representative expression out of each rule's
// rule_config: prom_ql for the metric rule, the key/op/value blurb for host.
func TestAlertRuleExprSummary(t *testing.T) {
	rules := parseAlertPack(t, alertPackFixture)

	if got := alertRuleExprSummary(&rules[0]); got != "cpu_usage_active > 80" {
		t.Fatalf("prom summary = %q, want %q", got, "cpu_usage_active > 80")
	}
	if got := alertRuleExprSummary(&rules[1]); got != "offset > [300]" {
		t.Fatalf("host summary = %q, want %q", got, "offset > [300]")
	}
}

// importedAlertRuleCard mirrors create_alert_rule's single-rule payload so the
// FE renders an imported rule identically: the prometheus rule carries the
// bound datasource; the host (heartbeat) rule omits it.
func TestImportedAlertRuleCard(t *testing.T) {
	rules := parseAlertPack(t, alertPackFixture)
	for i := range rules {
		prepareImportedAlertRule(&rules[i], 5, "alice", 7, 0, "", "")
		rules[i].Id = int64(100 + i) // simulate post-Add ids
	}

	promCard := importedAlertRuleCard(&rules[0], "Default Busi Group", 7, "prom")
	if promCard["id"] != int64(100) || promCard["name"] != "CPU high" {
		t.Fatalf("prom card id/name wrong: %#v", promCard)
	}
	if promCard["group_id"] != int64(5) || promCard["group_name"] != "Default Busi Group" {
		t.Fatalf("prom card group wrong: %#v", promCard)
	}
	if promCard["datasource_id"] != int64(7) || promCard["datasource_name"] != "prom" {
		t.Fatalf("prom card datasource wrong: %#v", promCard)
	}
	// severity + expression come from rule_config.queries, not the (zero) top-level columns.
	if promCard["severity"] != 2 {
		t.Fatalf("prom card severity = %v, want 2 (from rule_config.queries)", promCard["severity"])
	}
	if promCard["prom_ql"] != "cpu_usage_active > 80" {
		t.Fatalf("prom card prom_ql = %v, want the query expression", promCard["prom_ql"])
	}

	// Host rule: no datasource binding even though a dsId was passed; severity
	// comes from triggers, condition from the query.
	hostCard := importedAlertRuleCard(&rules[1], "Default Busi Group", 7, "prom")
	if _, ok := hostCard["datasource_id"]; ok {
		t.Fatalf("host card should omit datasource_id: %#v", hostCard)
	}
	if _, ok := hostCard["datasource_name"]; ok {
		t.Fatalf("host card should omit datasource_name: %#v", hostCard)
	}
	if hostCard["severity"] != 1 {
		t.Fatalf("host card severity = %v, want 1 (from triggers)", hostCard["severity"])
	}
	if hostCard["prom_ql"] != "offset > [300]" {
		t.Fatalf("host card condition = %v, want the query blurb", hostCard["prom_ql"])
	}
}

// A multi-tier prometheus rule (the real-world shape: one query per severity,
// threshold baked into the expression) must surface the MOST severe tier and
// pair its severity with its own expression.
func TestImportedRuleHeadline_MultiTierPicksMostSevere(t *testing.T) {
	const fixture = `[{"name":"Disk low","cate":"prometheus","prod":"metric",
	  "rule_config":{"queries":[
	    {"prom_ql":"disk_used_percent > 90 and disk_total < 200","severity":3},
	    {"prom_ql":"disk_used_percent > 95 and disk_total < 200","severity":2},
	    {"prom_ql":"disk_used_percent > 99 and disk_total < 200","severity":1}]}}]`
	rules := parseAlertPack(t, fixture)

	sev, expr := importedRuleHeadline(&rules[0])
	if sev != 1 {
		t.Fatalf("severity = %d, want 1 (most severe tier)", sev)
	}
	if expr != "disk_used_percent > 99 and disk_total < 200" {
		t.Fatalf("expr = %q, want the severity-1 tier's expression (paired)", expr)
	}
}

// disabled=1 keeps the imported rules off; name prefix/suffix are applied.
func TestPrepareImportedAlertRule_DisabledAndRename(t *testing.T) {
	rules := parseAlertPack(t, alertPackFixture)
	prepareImportedAlertRule(&rules[0], 5, "carol", 7, 1, "[AI] ", "-v2")

	if rules[0].Disabled != 1 {
		t.Fatalf("disabled = %d, want 1", rules[0].Disabled)
	}
	if rules[0].Name != "[AI] CPU high-v2" {
		t.Fatalf("name = %q, want %q", rules[0].Name, "[AI] CPU high-v2")
	}
}
