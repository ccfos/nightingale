package models_test

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestAlertRuleVerify_Severity(t *testing.T) {
	tests := []struct {
		name       string
		cate       string
		ruleConfig string
		wantErr    bool
	}{
		{"prometheus v1 query severity 1", models.PROMETHEUS, `{"queries":[{"prom_ql":"up == 0","severity":1}]}`, false},
		{"prometheus v1 query severity 3", models.PROMETHEUS, `{"queries":[{"prom_ql":"up == 0","severity":3}]}`, false},
		{"prometheus v1 query severity 0", models.PROMETHEUS, `{"queries":[{"prom_ql":"up == 0","severity":0}]}`, true},
		{"prometheus v1 query severity 4", models.PROMETHEUS, `{"queries":[{"prom_ql":"up == 0","severity":4}]}`, true},
		{"prometheus legacy config severity", models.PROMETHEUS, `{"severity":2}`, false},
		{"prometheus legacy config invalid severity", models.PROMETHEUS, `{"severity":-1}`, true},
		{"unused legacy severity may remain zero", models.PROMETHEUS, `{"severity":0,"queries":[{"prom_ql":"up == 0","severity":2}]}`, false},
		{"unused legacy severity cannot be another invalid value", models.PROMETHEUS, `{"severity":4,"queries":[{"prom_ql":"up == 0","severity":2}]}`, true},
		{"prometheus v2 trigger severity", models.PROMETHEUS, `{"version":"v2","triggers":[{"exp":"$A > 0","severity":1}]}`, false},
		{"prometheus v2 invalid trigger severity", models.PROMETHEUS, `{"version":"v2","triggers":[{"exp":"$A > 0","severity":9}]}`, true},
		{"host trigger severity", models.HOST, `{"triggers":[{"severity":3}]}`, false},
		{"host invalid trigger severity", models.HOST, `{"triggers":[{"severity":0}]}`, true},
		{"enabled nodata trigger severity", models.CLICKHOUSE, `{"nodata_trigger":{"enable":true,"severity":2}}`, false},
		{"enabled nodata trigger invalid severity", models.CLICKHOUSE, `{"nodata_trigger":{"enable":true,"severity":0}}`, true},
		{"disabled nodata trigger may retain zero", models.CLICKHOUSE, `{"nodata_trigger":{"enable":false,"severity":0}}`, false},
		{"enabled anomaly trigger invalid severity", models.CLICKHOUSE, `{"anomaly_trigger":{"enable":true,"severity":4}}`, true},
		{"empty config has no active severity", models.PROMETHEUS, `{}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &models.AlertRule{Name: "testrule", Cate: tt.cate, RuleConfig: tt.ruleConfig}
			err := ar.Verify()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "severity must be 1, 2 or 3") {
					t.Fatalf("expected severity error, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestAlertRuleVerify_LegacyTopLevelSeverity(t *testing.T) {
	for _, severity := range []int{-1, 4} {
		ar := &models.AlertRule{
			Name:       "testrule",
			Cate:       models.PROMETHEUS,
			Severity:   severity,
			RuleConfig: `{"queries":[{"prom_ql":"up == 0","severity":2}]}`,
		}
		if err := ar.Verify(); err == nil || !strings.Contains(err.Error(), "severity must be 1, 2 or 3") {
			t.Fatalf("severity %d: expected severity error, got: %v", severity, err)
		}
	}
}

func TestAlertRuleVerify_RequiredExpressions(t *testing.T) {
	tests := []struct {
		name       string
		cate       string
		ruleConfig string
		wantErr    string
	}{
		{"prometheus v1 blank prom_ql", models.PROMETHEUS, `{"queries":[{"prom_ql":"","severity":2}]}`, "prom_ql is blank"},
		{"prometheus v1 whitespace prom_ql", models.PROMETHEUS, `{"queries":[{"prom_ql":"  ","severity":2}]}`, "prom_ql is blank"},
		{"loki v1 blank prom_ql", models.LOKI, `{"queries":[{"prom_ql":"","severity":2}]}`, "prom_ql is blank"},
		{"prometheus v2 blank exp", models.PROMETHEUS, `{"version":"v2","triggers":[{"exp":"","severity":2}]}`, "exp is blank"},
		{"clickhouse blank exp", models.CLICKHOUSE, `{"triggers":[{"exp":"","severity":2}]}`, "exp is blank"},
		{"clickhouse valid exp", models.CLICKHOUSE, `{"triggers":[{"exp":"$A > 0","severity":2}]}`, ""},
		{"disabled threshold conditions allow blank exp", models.CLICKHOUSE, `{"exp_trigger_disable":true,"triggers":[{"exp":"","severity":2}],"nodata_trigger":{"enable":true,"severity":2}}`, ""},
		{"host triggers have no exp", models.HOST, `{"triggers":[{"type":"target_miss","duration":60,"severity":2}]}`, ""},
		// n9e-plus 老智能告警规则（prod=anomaly）经 AlertRuleModifyHook 迁移后的 GET 返回形态：
		// queries 是算法输入（无 prom_ql/severity），事件 severity 由 anomaly_trigger 承载
		{"anomaly migrated queries are exempt", models.PROMETHEUS, `{"version":"","queries":[{"ref":"A","query":"cpu_usage_active","unit":""}],"triggers":null,"anomaly_trigger":{"enable":true,"severity":2,"algorithm":"holt-winters"}}`, ""},
		{"anomaly migrated shape still validates anomaly severity", models.PROMETHEUS, `{"queries":[{"ref":"A","query":"cpu_usage_active"}],"anomaly_trigger":{"enable":true,"severity":4}}`, "severity must be 1, 2 or 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &models.AlertRule{Name: "testrule", Cate: tt.cate, RuleConfig: tt.ruleConfig}
			err := ar.Verify()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// Host rules are dispatched by prod; the cate may be blank (Verify then defaults
// it to prometheus) or stale, and neither must route them into the prom v1 checks.
func TestAlertRuleVerify_HostRuleClassifiedByProd(t *testing.T) {
	hostConfig := `{"queries":[{"key":"group_ids","op":"==","values":[1]}],"triggers":[{"type":"target_miss","duration":60,"severity":2}]}`
	tests := []struct {
		name       string
		cate       string
		ruleConfig string
		wantErr    string
	}{
		{"blank cate", "", hostConfig, ""},
		{"stale prometheus cate", models.PROMETHEUS, hostConfig, ""},
		{"blank cate still validates trigger severity", "", `{"queries":[{"key":"group_ids","op":"==","values":[1]}],"triggers":[{"type":"target_miss","duration":60,"severity":0}]}`, "severity must be 1, 2 or 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &models.AlertRule{Name: "testrule", Prod: models.HOST, Cate: tt.cate, RuleConfig: tt.ruleConfig}
			err := ar.Verify()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestAlertRuleUpdateColumn_RejectsInvalidSeverity(t *testing.T) {
	ar := &models.AlertRule{}
	for _, severity := range []interface{}{float64(0), float64(4), float64(1.5), "2"} {
		if err := ar.UpdateColumn(nil, "severity", severity); err == nil || !strings.Contains(err.Error(), "severity must be 1, 2 or 3") {
			t.Fatalf("severity %v: expected severity error, got: %v", severity, err)
		}
	}
}

func TestAlertRuleVerify_EffectiveTimeSpan(t *testing.T) {
	base := func() *models.AlertRule {
		return &models.AlertRule{Name: "testrule", RuleConfig: "{}"}
	}
	tests := []struct {
		name    string
		mutate  func(ar *models.AlertRule)
		wantErr bool
	}{
		{"no time span configured", func(ar *models.AlertRule) {}, false},
		{"days of week without time (panic case)", func(ar *models.AlertRule) {
			ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2"}}
		}, true},
		{"time without days of week", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00"}
			ar.EnableEtimesJSON = []string{"09:59"}
		}, true},
		{"count mismatch between start and end", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00", "10:00"}
			ar.EnableEtimesJSON = []string{"09:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}, {"2"}}
		}, true},
		{"time configured but empty days group", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00"}
			ar.EnableEtimesJSON = []string{"09:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{}}
		}, true},
		{"valid single span", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00"}
			ar.EnableEtimesJSON = []string{"09:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2"}}
		}, false},
		{"valid multiple spans", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00", "10:00"}
			ar.EnableEtimesJSON = []string{"09:59", "23:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}, {"2"}}
		}, false},
		{"valid legacy single time field", func(ar *models.AlertRule) {
			ar.EnableStimeJSON = "00:00"
			ar.EnableEtimeJSON = "09:59"
			ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2"}}
		}, false},
		{"valid legacy singular days-of-week field", func(ar *models.AlertRule) {
			// 内置告警模板（如 Net_Response、Kubernetes apiserver/kubelet）的形态：
			// 只带单数 enable_stime/enable_etime/enable_days_of_week，复数数组全空。
			ar.EnableStimeJSON = "00:00"
			ar.EnableEtimeJSON = "23:59"
			ar.EnableDaysOfWeekJSON = []string{"1", "2", "3", "4", "5", "6", "0"}
		}, false},
		{"unconfigured rule from DB2FE empty group", func(ar *models.AlertRule) {
			ar.EnableDaysOfWeeksJSON = [][]string{{}}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := base()
			tt.mutate(ar)
			err := ar.Verify()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestAlertRuleVerify_EffectiveTimeFormat(t *testing.T) {
	base := func() *models.AlertRule {
		return &models.AlertRule{Name: "testrule", RuleConfig: "{}"}
	}
	tests := []struct {
		name    string
		mutate  func(ar *models.AlertRule)
		wantErr bool
	}{
		// 前端 moment 拿到脏数据后格式化出来的值，就是这条 bug 的实际入参形态。
		{"moment invalid date", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"Invalid date"}
			ar.EnableEtimesJSON = []string{"23:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, true},
		{"invalid end time", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00"}
			ar.EnableEtimesJSON = []string{"Invalid"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, true},
		{"hour and minute out of range", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"25:99"}
			ar.EnableEtimesJSON = []string{"23:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, true},
		{"missing leading zero", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"8:00"}
			ar.EnableEtimesJSON = []string{"18:00"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, true},
		{"empty time string", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{""}
			ar.EnableEtimesJSON = []string{""}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, true},
		{"invalid time in second span", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00", "10:00"}
			ar.EnableEtimesJSON = []string{"09:59", "24:30"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}, {"2"}}
		}, true},
		// 24:00 只作为结束时间成立：触发时刻最大 23:59，恒小于 24:00，即生效到当日结束。
		{"start time 24:00 is rejected", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"24:00"}
			ar.EnableEtimesJSON = []string{"23:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, true},
		{"invalid legacy single time field", func(ar *models.AlertRule) {
			ar.EnableStimeJSON = "Invalid"
			ar.EnableEtimeJSON = "23:59"
			ar.EnableDaysOfWeekJSON = []string{"1", "2"}
		}, true},
		{"valid cross-day span", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"22:00"}
			ar.EnableEtimesJSON = []string{"06:00"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, false},
		{"valid span ending at 24:00", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"02:00"}
			ar.EnableEtimesJSON = []string{"24:00"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1"}}
		}, false},
		{"valid all day span", func(ar *models.AlertRule) {
			ar.EnableStimesJSON = []string{"00:00"}
			ar.EnableEtimesJSON = []string{"23:59"}
			ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2", "3", "4", "5", "6", "0"}}
		}, false},
		{"valid legacy single time field", func(ar *models.AlertRule) {
			ar.EnableStimeJSON = "00:00"
			ar.EnableEtimeJSON = "23:59"
			ar.EnableDaysOfWeekJSON = []string{"1", "2"}
		}, false},
		{"no time span configured", func(ar *models.AlertRule) {}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := base()
			tt.mutate(ar)
			err := ar.Verify()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}
