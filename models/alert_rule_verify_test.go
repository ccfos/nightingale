package models_test

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// 覆盖 AlertRule.Verify 对「生效时段」的一致性校验：
// 开始时间、结束时间、生效星期三者段数必须一致，且配置了时段时星期不能为空。
func TestAlertRuleVerify_EffectiveTimeSpan(t *testing.T) {
	base := func() *models.AlertRule {
		return &models.AlertRule{Name: "testrule", RuleConfig: "{}"}
	}

	tests := []struct {
		name    string
		mutate  func(ar *models.AlertRule)
		wantErr bool
	}{
		{
			name:    "no time span configured",
			mutate:  func(ar *models.AlertRule) {},
			wantErr: false,
		},
		{
			name: "days of week without time (panic case)",
			mutate: func(ar *models.AlertRule) {
				ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2"}}
			},
			wantErr: true,
		},
		{
			name: "time without days of week",
			mutate: func(ar *models.AlertRule) {
				ar.EnableStimesJSON = []string{"00:00"}
				ar.EnableEtimesJSON = []string{"09:59"}
			},
			wantErr: true,
		},
		{
			name: "count mismatch between start and end",
			mutate: func(ar *models.AlertRule) {
				ar.EnableStimesJSON = []string{"00:00", "10:00"}
				ar.EnableEtimesJSON = []string{"09:59"}
				ar.EnableDaysOfWeeksJSON = [][]string{{"1"}, {"2"}}
			},
			wantErr: true,
		},
		{
			name: "empty days group",
			mutate: func(ar *models.AlertRule) {
				ar.EnableStimesJSON = []string{"00:00"}
				ar.EnableEtimesJSON = []string{"09:59"}
				ar.EnableDaysOfWeeksJSON = [][]string{{}}
			},
			wantErr: true,
		},
		{
			name: "valid single span",
			mutate: func(ar *models.AlertRule) {
				ar.EnableStimesJSON = []string{"00:00"}
				ar.EnableEtimesJSON = []string{"09:59"}
				ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2"}}
			},
			wantErr: false,
		},
		{
			name: "valid multiple spans",
			mutate: func(ar *models.AlertRule) {
				ar.EnableStimesJSON = []string{"00:00", "10:00"}
				ar.EnableEtimesJSON = []string{"09:59", "23:59"}
				ar.EnableDaysOfWeeksJSON = [][]string{{"1"}, {"2"}}
			},
			wantErr: false,
		},
		{
			name: "valid legacy single time field",
			mutate: func(ar *models.AlertRule) {
				ar.EnableStimeJSON = "00:00"
				ar.EnableEtimeJSON = "09:59"
				ar.EnableDaysOfWeeksJSON = [][]string{{"1", "2"}}
			},
			wantErr: false,
		},
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
