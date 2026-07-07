package models_test

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

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
