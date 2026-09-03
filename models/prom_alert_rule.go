package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
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
		return 60
	}

	if duration.Seconds() == 0 {
		duration = 60 * time.Second
	}

	return int(duration.Seconds())
}

func ConvertAlert(rule PromRule, interval string, datasouceQueries []DatasourceQuery, disabled int) AlertRule {
	annotations := rule.Annotations
	appendTags := []string{}
	severity := 2

	ruleName := rule.Alert
	if len(rule.Labels) > 0 {
		for k, v := range rule.Labels {
			if k != "severity" {
				appendTags = append(appendTags, fmt.Sprintf("%s=%s", strings.ReplaceAll(k, " ", ""), strings.ReplaceAll(v, " ", "")))
			} else {
				switch v {
				case "critical", "Critical", "CRITICAL", "error", "Error", "ERROR", "fatal", "Fatal", "FATAL", "page", "Page", "PAGE", "sev1", "SEV1", "Severity1", "severity1", "SEVERITY1":
					severity = 1
				case "warning", "Warning", "WARNING", "warn", "Warn", "WARN", "sev2", "SEV2", "Severity2", "severity2", "SEVERITY2":
					severity = 2
				case "info", "Info", "INFO", "notice", "Notice", "NOTICE", "sev3", "SEV3", "Severity3", "severity3", "SEVERITY3":
					severity = 3
				}
				ruleName += "-" + v
			}
		}
	}

	ar := AlertRule{
		Name:              rule.Alert,
		Severity:          severity,
		Disabled:          disabled,
		PromForDuration:   convertInterval(rule.For),
		PromQl:            rule.Expr,
		CronPattern:       fmt.Sprintf("@every %ds", convertInterval(interval)),
		EnableInBG:        AlertRuleEnableInGlobalBG,
		NotifyRecovered:   AlertRuleNotifyRecovered,
		NotifyRepeatStep:  AlertRuleNotifyRepeatStep60Min,
		RecoverDuration:   AlertRuleRecoverDuration0Sec,
		AnnotationsJSON:   annotations,
		AppendTagsJSON:    appendTags,
		DatasourceQueries: datasouceQueries,
		NotifyVersion:     1,
		NotifyRuleIds:     []int64{},
		// 显式初始化为空切片：FE2DB 把 nil 切片序列化成 JSON null，
		// 前端 / 后续迁移逻辑若对 null vs [] 处理不一致会显示异常。
		// 这三个字段 deprecated 但 schema 还在，统一落 "[]" 更稳。
		// 同样解决 HTTP /alert-rules-import 那条路径同样的隐患（共用 ConvertAlert）。
		NotifyChannelsJSON: []string{},
		NotifyGroupsJSON:   []string{},
		CallbacksJSON:      []string{},
	}

	return ar
}

// ParsePromRuleYAML accepts the three shapes that the import endpoint
// historically tolerated and returns a single normalised []PromRuleGroup:
//  1. Standard Prometheus file with top-level `groups:`.
//  2. A bare list of rules (treated as one synthetic group "imported_rules").
//  3. A single rule object (wrapped the same way).
//
// Lives in the model layer so both center/router (HTTP handler) and the
// aiagent builtin tool (import/preview) decode identically — there's no good
// reason for two parsers to drift.
func ParsePromRuleYAML(payload string) ([]PromRuleGroup, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, fmt.Errorf("payload is empty")
	}

	var pr struct {
		Groups []PromRuleGroup `yaml:"groups"`
	}
	if err := yaml.Unmarshal([]byte(payload), &pr); err == nil && len(pr.Groups) > 0 {
		return pr.Groups, nil
	}

	var rules []PromRule
	if err := yaml.Unmarshal([]byte(payload), &rules); err == nil && len(rules) > 0 {
		return []PromRuleGroup{{Name: "imported_rules", Rules: rules}}, nil
	}

	var single PromRule
	if err := yaml.Unmarshal([]byte(payload), &single); err != nil {
		return nil, fmt.Errorf("invalid yaml format: %v", err)
	}
	if single.Alert == "" && single.Record == "" {
		return nil, fmt.Errorf("input yaml is empty or invalid")
	}
	return []PromRuleGroup{{Name: "imported_rules", Rules: []PromRule{single}}}, nil
}

func DealPromGroup(promRule []PromRuleGroup, dataSourceQueries []DatasourceQuery, disabled int) []AlertRule {
	var alertRules []AlertRule

	for _, group := range promRule {
		interval := group.Interval
		if interval == "" {
			interval = "60s"
		}
		for _, rule := range group.Rules {
			if rule.Alert != "" {
				alertRules = append(alertRules,
					ConvertAlert(rule, interval, dataSourceQueries, disabled))
			}
		}
	}

	return alertRules
}
