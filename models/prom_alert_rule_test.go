package models_test

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"gopkg.in/yaml.v2"
)

func TestConvertAlert(t *testing.T) {
	jobMissing := []models.PromRule{}
	err := yaml.Unmarshal([]byte(`  - alert: PrometheusJobMissing
    expr: absent(up{job="prometheus"})
    for: 1m
    labels:
      severity: warning
    annotations:
      summary: Prometheus job missing (instance {{ $labels.instance }})
      description: "A Prometheus job has disappeared\n  VALUE = {{ $value }}\n  LABELS = {{ $labels }}"`), &jobMissing)
	if err != nil {
		t.Fatalf("unmarshal PrometheusJobMissing: %v", err)
	}
	t.Logf("jobMissing: %+v", jobMissing[0])
	convJobMissing := models.ConvertAlert(jobMissing[0], "30s", []models.DatasourceQuery{}, 0)
	if convJobMissing.CronPattern != "@every 30s" {
		t.Errorf("CronPattern is expected to be @every 30s, but got %s",
			convJobMissing.CronPattern)
	}
	if convJobMissing.PromForDuration != 60 {
		t.Errorf("PromForDuration is expected to be 60, but got %d",
			convJobMissing.PromForDuration)
	}
	if convJobMissing.Severity != models.SeverityWarning {
		t.Errorf("Severity is expected to be %d, but got %d", models.SeverityWarning, convJobMissing.Severity)
	}

	ruleEvaluationSlow := []models.PromRule{}
	if err := yaml.Unmarshal([]byte(`  - alert: PrometheusRuleEvaluationSlow
    expr: prometheus_rule_group_last_duration_seconds > prometheus_rule_group_interval_seconds
    for: 180s
    labels:
      severity: info
    annotations:
      summary: Prometheus rule evaluation slow (instance {{ $labels.instance }})
      description: "Prometheus rule evaluation took more time than the scheduled interval. It indicates a slower storage backend access or too complex query.\n  VALUE = {{ $value }}\n  LABELS = {{ $labels }}"
`), &ruleEvaluationSlow); err != nil {
		t.Fatalf("unmarshal PrometheusRuleEvaluationSlow: %v", err)
	}
	t.Logf("ruleEvaluationSlow: %+v", ruleEvaluationSlow[0])
	convRuleEvaluationSlow := models.ConvertAlert(ruleEvaluationSlow[0], "1m", []models.DatasourceQuery{}, 0)
	if convRuleEvaluationSlow.CronPattern != "@every 60s" {
		t.Errorf("CronPattern is expected to be @every 60s, but got %s",
			convRuleEvaluationSlow.CronPattern)
	}
	if convRuleEvaluationSlow.PromForDuration != 180 {
		t.Errorf("PromForDuration is expected to be 180, but got %d",
			convRuleEvaluationSlow.PromForDuration)
	}
	if convRuleEvaluationSlow.Severity != models.SeverityNotice {
		t.Errorf("Severity is expected to be %d, but got %d", models.SeverityNotice, convRuleEvaluationSlow.Severity)
	}

	targetMissing := []models.PromRule{}
	if err := yaml.Unmarshal([]byte(`  - alert: PrometheusTargetMissing
    expr: up == 0
    for: 1.5m
    labels:
      severity: critical
    annotations:
      summary: Prometheus target missing (instance {{ $labels.instance }})
      description: "A Prometheus target has disappeared. An exporter might be crashed.\n  VALUE = {{ $value }}\n  LABELS = {{ $labels }}"
`), &targetMissing); err != nil {
		t.Fatalf("unmarshal PrometheusTargetMissing: %v", err)
	}
	t.Logf("targetMissing: %+v", targetMissing[0])
	convTargetMissing := models.ConvertAlert(targetMissing[0], "1h", []models.DatasourceQuery{}, 0)
	if convTargetMissing.CronPattern != "@every 3600s" {
		t.Errorf("CronPattern is expected to be @every 3600s, but got %s",
			convTargetMissing.CronPattern)
	}
	if convTargetMissing.PromForDuration != 90 {
		t.Errorf("PromForDuration is expected to be 90, but got %d",
			convTargetMissing.PromForDuration)
	}
	if convTargetMissing.Severity != models.SeverityEmergency {
		t.Errorf("Severity is expected to be %d, but got %d", models.SeverityEmergency, convTargetMissing.Severity)
	}
}
