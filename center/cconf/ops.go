package cconf

import (
	"fmt"
	"path"

	"github.com/toolkits/pkg/file"
	"gopkg.in/yaml.v2"
)

var Operations = Operation{}

type Operation struct {
	Ops []Ops `yaml:"ops"`
}

type Ops struct {
	Name  string     `yaml:"name" json:"name"`
	Cname string     `yaml:"cname" json:"cname"`
	Ops   []SingleOp `yaml:"ops" json:"ops"`
}

// SingleOp Name 为 op 名称；Cname 为展示名称，默认英文
type SingleOp struct {
	Name  string `yaml:"name" json:"name"`
	Cname string `yaml:"cname" json:"cname"`
}

func TransformNames(name []string, nameToName map[string]string) []string {
	var ret []string
	for _, n := range name {
		if v, has := nameToName[n]; has {
			ret = append(ret, v)
		}
	}
	return ret
}

func LoadOpsYaml(configDir string, opsYamlFile string) error {
	fp := opsYamlFile
	if fp == "" {
		fp = path.Join(configDir, "ops.yaml")
	}
	if !file.IsExist(fp) {
		return nil
	}

	hash, _ := file.MD5(fp)
	if hash == "2f91a9ed265cf2024e266dc1d538ee77" {
		// ops.yaml 是老的默认文件，删除
		file.Remove(fp)
		return nil
	}

	return file.ReadYaml(fp, &Operations)
}

func GetAllOps(ops []Ops) []SingleOp {
	var ret []SingleOp
	for _, op := range ops {
		ret = append(ret, op.Ops...)
	}
	return ret
}

func MergeOperationConf() error {
	var opsBuiltIn Operation
	err := yaml.Unmarshal([]byte(builtInOps), &opsBuiltIn)
	if err != nil {
		return fmt.Errorf("cannot parse builtInOps: %s", err.Error())
	}
	configOpsMap := make(map[string]struct{})
	for _, op := range Operations.Ops {
		configOpsMap[op.Name] = struct{}{}
	}
	//If the opBu.Name is not a constant in the target (Operations.Ops), add Ops from the built-in options
	for _, opBu := range opsBuiltIn.Ops {
		if _, has := configOpsMap[opBu.Name]; !has {
			Operations.Ops = append(Operations.Ops, opBu)
		}
	}
	return nil
}

const (
	builtInOps = `
ops:
- name: dashboards
  cname: Dashboards
  ops:
    - name: "/dashboards"
      cname: View Dashboards
    - name: "/dashboards/add"
      cname: Add Dashboard
    - name: "/dashboards/put"
      cname: Modify Dashboard
    - name: "/dashboards/del"
      cname: Delete Dashboard
    - name: "/embedded-dashboards/put"
      cname: Modify Embedded Dashboard
    - name: "/embedded-dashboards"
      cname: View Embedded Dashboard
    - name: "/public-dashboards"
      cname: View Public Dashboard

- name: metric
  cname: Time Series Metrics
  ops:
    - name: "/metric/explorer"
      cname: View Metric Data
    - name: "/object/explorer"
      cname: View Object Data

- name: builtin-metrics
  cname: Metric Views
  ops:
    - name: "/metrics-built-in"
      cname: View Built-in Metrics
    - name: "/builtin-metrics/add"
      cname: Add Built-in Metric
    - name: "/builtin-metrics/put"
      cname: Modify Built-in Metric
    - name: "/builtin-metrics/del"
      cname: Delete Built-in Metric

- name: recording-rules
  cname: Recording Rule Management
  ops:
    - name: "/recording-rules"
      cname: View Recording Rules
    - name: "/recording-rules/add"
      cname: Add Recording Rule
    - name: "/recording-rules/put"
      cname: Modify Recording Rule
    - name: "/recording-rules/del"
      cname: Delete Recording Rule

- name: log
  cname: Log Analysis
  ops:
    - name: "/log/explorer"
      cname: View Logs
    - name: "/log/index-patterns"
      cname: View Index Patterns

- name: alert
  cname: Alert Rules
  ops:
    - name: "/alert-rules"
      cname: View Alert Rules
    - name: "/alert-rules/add"
      cname: Add Alert Rule
    - name: "/alert-rules/put"
      cname: Modify Alert Rule
    - name: "/alert-rules/del"
      cname: Delete Alert Rule

- name: alert-mutes
  cname: Alert Silence Management
  ops:
    - name: "/alert-mutes"
      cname: View Alert Silences
    - name: "/alert-mutes/add"
      cname: Add Alert Silence
    - name: "/alert-mutes/put"
      cname: Modify Alert Silence
    - name: "/alert-mutes/del"
      cname: Delete Alert Silence
  
- name: alert-subscribes
  cname: Alert Subscription Management
  ops:
    - name: "/alert-subscribes"
      cname: View Alert Subscriptions
    - name: "/alert-subscribes/add"
      cname: Add Alert Subscription
    - name: "/alert-subscribes/put"
      cname: Modify Alert Subscription
    - name: "/alert-subscribes/del"
      cname: Delete Alert Subscription

- name: alert-events  
  cname: Alert Event Management
  ops:
    - name: "/alert-cur-events"
      cname: View Current Alerts
    - name: "/alert-cur-events/del"
      cname: Delete Current Alert
    - name: "/alert-his-events"
      cname: View Historical Alerts

- name: notification
  cname: Alert Notification
  ops:
    - name: "/help/notification-settings"
      cname: View Notification Settings
    - name: "/help/notification-tpls"
      cname: View Notification Templates

- name: job
  cname: Task Management
  ops:
    - name: "/job-tpls"
      cname: View Task Templates
    - name: "/job-tpls/add"
      cname: Add Task Template
    - name: "/job-tpls/put"
      cname: Modify Task Template
    - name: "/job-tpls/del"
      cname: Delete Task Template
    - name: "/job-tasks"
      cname: View Task Instances
    - name: "/job-tasks/add"
      cname: Add Task Instance
    - name: "/job-tasks/put"
      cname: Modify Task Instance

- name: targets
  cname: Infrastructure
  ops:
    - name: "/targets"
      cname: View Objects
    - name: "/targets/add"
      cname: Add Object
    - name: "/targets/put"
      cname: Modify Object
    - name: "/targets/del"
      cname: Delete Object
    - name: "/targets/bind"
      cname: Bind Object

- name: user
  cname: User Management
  ops:
    - name: "/users"
      cname: View User List
    - name: "/user-groups"
      cname: View User Groups
    - name: "/user-groups/add"
      cname: Add User Group
    - name: "/user-groups/put"
      cname: Modify User Group
    - name: "/user-groups/del"
      cname: Delete User Group

- name: busi-groups
  cname: Business Group Management
  ops:
    - name: "/busi-groups"
      cname: View Business Groups
    - name: "/busi-groups/add"
      cname: Add Business Group
    - name: "/busi-groups/put"
      cname: Modify Business Group
    - name: "/busi-groups/del"
      cname: Delete Business Group

- name: permissions
  cname: Permission Management
  ops:
    - name: "/permissions"
      cname: View Permission Settings

- name: built-in-components
  cname: Template Center
  ops:
    - name: "/built-in-components"
      cname: View Built-in Components
    - name: "/built-in-components/add"
      cname: Add Built-in Component
    - name: "/built-in-components/put"
      cname: Modify Built-in Component
    - name: "/built-in-components/del"
      cname: Delete Built-in Component

- name: datasource
  cname: Data Source Management
  ops:
    - name: "/help/source"
      cname: View Data Source Configuration

- name: system
  cname: System Information
  ops:
    - name: "/help/variable-configs"
      cname: View Variable Configuration
    - name: "/help/version"
      cname: View Version Information
    - name: "/help/servers"
      cname: View Server Information
    - name: "/help/sso"
      cname: View SSO Configuration
    - name: "/site-settings"
      cname: View Site Settings
`
)
