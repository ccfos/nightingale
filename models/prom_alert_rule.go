package models

import (
	"fmt"
	"time"

	"github.com/toolkits/pkg/logger"
)

type PromRule struct {
	Alert       string            `yaml:"alert,omitempty" json:"alert,omitempty"`             // 报警规则的名称
	Record      string            `yaml:"record,omitempty" json:"record,omitempty"`           // 记录规则的名称
	Expr        string            `yaml:"expr,omitempty" json:"expr,omitempty"`               // PromQL 表达式
	For         string            `yaml:"for,omitempty" json:"for,omitempty"`                 // 告警的等待时间
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"` // 规则的注释信息
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`           // 规则的标签信息
}

type PromRuleGroup struct {
	Name     string     `yaml:"name"`
	Rules    []PromRule `yaml:"rules"`
	Interval string     `yaml:"interval,omitempty"`
}

func convertInterval(interval string) int {
	duration, err := time.ParseDuration(interval)
	if err != nil {
		logger.Errorf("Error parsing interval `%s`, err: %v", interval, err)
		return 0
	}
	return int(duration.Seconds())
}

func ConvertAlert(rule PromRule, interval string, datasouceIds []int64, disabled int) AlertRule {
	annotations := rule.Annotations
	appendTags := []string{}
	severity := 2

	if len(rule.Labels) > 0 {
		for k, v := range rule.Labels {
			if k != "severity" {
				appendTags = append(appendTags, fmt.Sprintf("%s=%s", k, v))
			} else {
				switch v {
				case "critical":
					severity = 1
				case "warning":
					severity = 2
				case "info":
					severity = 3
				}
			}
		}
	}

	return AlertRule{
		Name:              rule.Alert,
		Severity:          severity,
		DatasourceIdsJson: datasouceIds,
		Disabled:          disabled,
		PromForDuration:   convertInterval(rule.For),
		PromQl:            rule.Expr,
		PromEvalInterval:  convertInterval(interval),
		EnableStimeJSON:   "00:00",
		EnableEtimeJSON:   "23:59",
		EnableDaysOfWeekJSON: []string{
			"1", "2", "3", "4", "5", "6", "0",
		},
		EnableInBG:       AlertRuleEnableInGlobalBG,
		NotifyRecovered:  AlertRuleNotifyRecovered,
		NotifyRepeatStep: AlertRuleNotifyRepeatStep60Min,
		RecoverDuration:  AlertRuleRecoverDuration0Sec,
		AnnotationsJSON:  annotations,
		AppendTagsJSON:   appendTags,
	}
}

func DealPromGroup(promRule []PromRuleGroup, dataSourceIds []int64, disabled int) []AlertRule {
	var alertRules []AlertRule

	for _, group := range promRule {
		interval := group.Interval
		if interval == "" {
			interval = "15s"
		}
		for _, rule := range group.Rules {
			if rule.Alert != "" {
				alertRules = append(alertRules,
					ConvertAlert(rule, interval, dataSourceIds, disabled))
			}
		}
	}

	return alertRules
}
