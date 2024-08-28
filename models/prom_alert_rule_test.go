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
		t.Errorf("Failed to Unmarshal, err: %s", err)
	}
	t.Logf("jobMissing: %+v", jobMissing[0])
	convJobMissing := models.ConvertAlert(jobMissing[0], "30s", []int64{1}, 0)
	if convJobMissing.PromEvalInterval != 30 {
		t.Errorf("PromEvalInterval is expected to be 30, but got %d",
			convJobMissing.PromEvalInterval)
	}
	if convJobMissing.PromForDuration != 60 {
		t.Errorf("PromForDuration is expected to be 60, but got %d",
			convJobMissing.PromForDuration)
	}
	if convJobMissing.Severity != 2 {
		t.Errorf("Severity is expected to be 2, but got %d", convJobMissing.Severity)
	}

	ruleEvaluationSlow := []models.PromRule{}
	yaml.Unmarshal([]byte(`  - alert: PrometheusRuleEvaluationSlow
    expr: prometheus_rule_group_last_duration_seconds > prometheus_rule_group_interval_seconds
    for: 180s
    labels:
      severity: info
    annotations:
      summary: Prometheus rule evaluation slow (instance {{ $labels.instance }})
      description: "Prometheus rule evaluation took more time than the scheduled interval. It indicates a slower storage backend access or too complex query.\n  VALUE = {{ $value }}\n  LABELS = {{ $labels }}"
`), &ruleEvaluationSlow)
	t.Logf("ruleEvaluationSlow: %+v", ruleEvaluationSlow[0])
	convRuleEvaluationSlow := models.ConvertAlert(ruleEvaluationSlow[0], "1m", []int64{1}, 0)
	if convRuleEvaluationSlow.PromEvalInterval != 60 {
		t.Errorf("PromEvalInterval is expected to be 60, but got %d",
			convJobMissing.PromEvalInterval)
	}
	if convRuleEvaluationSlow.PromForDuration != 180 {
		t.Errorf("PromForDuration is expected to be 180, but got %d",
			convJobMissing.PromForDuration)
	}
	if convRuleEvaluationSlow.Severity != 3 {
		t.Errorf("Severity is expected to be 3, but got %d", convJobMissing.Severity)
	}

	targetMissing := []models.PromRule{}
	yaml.Unmarshal([]byte(`  - alert: PrometheusTargetMissing
    expr: up == 0
    for: 1.5m
    labels:
      severity: critical
    annotations:
      summary: Prometheus target missing (instance {{ $labels.instance }})
      description: "A Prometheus target has disappeared. An exporter might be crashed.\n  VALUE = {{ $value }}\n  LABELS = {{ $labels }}"
`), &targetMissing)
	t.Logf("targetMissing: %+v", targetMissing[0])
	convTargetMissing := models.ConvertAlert(targetMissing[0], "1h", []int64{1}, 0)
	if convTargetMissing.PromEvalInterval != 3600 {
		t.Errorf("PromEvalInterval is expected to be 3600, but got %d",
			convTargetMissing.PromEvalInterval)
	}
	if convTargetMissing.PromForDuration != 90 {
		t.Errorf("PromForDuration is expected to be 90, but got %d",
			convTargetMissing.PromForDuration)
	}
	if convTargetMissing.Severity != 1 {
		t.Errorf("Severity is expected to be 1, but got %d", convTargetMissing.Severity)
	}
}
