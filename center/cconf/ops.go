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
- name: Infrastructure
  cname: Infrastructure
  ops:
    - name: /targets
      cname: Host - View
    - name: /targets/put
      cname: Host - Modify
    - name: /targets/del
      cname: Host - Delete
    - name: /targets/bind
      cname: Host - Bind Uncategorized

- name: Explorer
  cname: Explorer
  ops:
    - name: /metric/explorer
      cname: Metrics Explorer
    - name: /object/explorer
      cname: Quick View
    - name: /metrics-built-in
      cname: Built-in Metric - View
    - name: /builtin-metrics/add
      cname: Built-in Metric - Add
    - name: /builtin-metrics/put
      cname: Built-in Metric - Modify
    - name: /builtin-metrics/del
      cname: Built-in Metric - Delete
    - name: /recording-rules
      cname: Recording Rule - View
    - name: /recording-rules/add
      cname: Recording Rule - Add
    - name: /recording-rules/put
      cname: Recording Rule - Modify
    - name: /recording-rules/del
      cname: Recording Rule - Delete
    - name: /log/explorer
      cname: Logs Explorer
    - name: /log/index-patterns # 前端有个管理索引模式的页面，所以需要一个权限点来控制，后面应该改成侧拉板
      cname: Index Pattern - View
    - name: /log/index-patterns/add
      cname: Index Pattern - Add
    - name: /log/index-patterns/put
      cname: Index Pattern - Modify
    - name: /log/index-patterns/del
      cname: Index Pattern - Delete
    - name: /dashboards
      cname: Dashboard - View
    - name: /dashboards/add
      cname: Dashboard - Add
    - name: /dashboards/put
      cname: Dashboard - Modify
    - name: /dashboards/del
      cname: Dashboard - Delete
    - name: /public-dashboards
      cname: Dashboard - View Public

- name: alerting
  cname: Alerting
  ops:
    - name: /alert-rules
      cname: Alerting Rule - View
    - name: /alert-rules/add
      cname: Alerting Rule - Add
    - name: /alert-rules/put
      cname: Alerting Rule - Modify
    - name: /alert-rules/del
      cname: Alerting Rule - Delete
    - name: /alert-mutes
      cname: Mutting Rule - View
    - name: /alert-mutes/add
      cname: Mutting Rule - Add
    - name: /alert-mutes/put
      cname: Mutting Rule - Modify
    - name: /alert-mutes/del
      cname: Mutting Rule - Delete
    - name: /alert-subscribes
      cname: Subscribing Rule - View
    - name: /alert-subscribes/add
      cname: Subscribing Rule - Add
    - name: /alert-subscribes/put
      cname: Subscribing Rule - Modify
    - name: /alert-subscribes/del
      cname: Subscribing Rule - Delete
    - name: /job-tpls
      cname: Self-healing-Script - View
    - name: /job-tpls/add
      cname: Self-healing-Script - Add
    - name: /job-tpls/put
      cname: Self-healing-Script - Modify
    - name: /job-tpls/del
      cname: Self-healing-Script - Delete
    - name: /job-tasks
      cname: Self-healing-Job - View
    - name: /job-tasks/add
      cname: Self-healing-Job - Add
    - name: /job-tasks/put
      cname: Self-healing-Job - Modify
    - name: /alert-cur-events
      cname: Active Event - View
    - name: /alert-cur-events/del
      cname: Active Event - Delete
    - name: /alert-his-events
      cname: Historical Event - View

- name: Notification
  cname: Notification
  ops:
    - name: /help/notification-settings # 用于控制老版本的通知设置菜单是否展示
      cname: Notification Settings - View
    - name: /help/notification-tpls # 用于控制老版本的通知模板菜单是否展示
      cname: Notification Templates - View

- name: Organization
  cname: Organization
  ops:
    - name: /users
      cname: User - View
    - name: /user-groups
      cname: Team - View
    - name: /user-groups/add
      cname: Team - Add
    - name: /user-groups/put
      cname: Team - Modify
    - name: /user-groups/del
      cname: Team - Delete
    - name: /busi-groups
      cname: Business Group - View
    - name: /busi-groups/add
      cname: Business Group - Add
    - name: /busi-groups/put
      cname: Business Group - Modify
    - name: /busi-groups/del
      cname: Business Group - Delete
    - name: /roles
      cname: Role - View

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

- name: message-templates
  cname: Message Templates
  ops:
    - name: "/notification-templates"
      cname: View Message Templates
    - name: "/notification-templates/add"
      cname: Add Message Templates
    - name: "/notification-templates/put"
      cname: Modify Message Templates
    - name: "/notification-templates/del"
      cname: Delete Message Templates

- name: notify-rules
  cname: Notify Rules
  ops:
    - name: "/notification-rules"
      cname: View Notify Rules
    - name: "/notification-rules/add"
      cname: Add Notify Rules
    - name: "/notification-rules/put"
      cname: Modify Notify Rules
    - name: "/notification-rules/del"
      cname: Delete Notify Rules

- name: event-pipelines
  cname: Event Pipelines
  ops:
    - name: "/event-pipelines"
      cname: View Event Pipelines
    - name: "/event-pipelines/add"
      cname: Add Event Pipeline
    - name: "/event-pipelines/put"
      cname: Modify Event Pipeline
    - name: "/event-pipelines/del"
      cname: Delete Event Pipeline

- name: notify-channels
  cname: Notify Channels
  ops:
    - name: "/notification-channels"
      cname: View Notify Channels
    - name: "/notification-channels/add"
      cname: Add Notify Channels
    - name: "/notification-channels/put"
      cname: Modify Notify Channels
    - name: "/notification-channels/del"
      cname: Delete Notify Channels

- name: embedded-product
  cname: Integrated Instrument Dashboard
  ops:
    - name: "/embedded-product"
      cname: View Embedded Product
    - name: "/embedded-product/add"
      cname: Add Embedded Product
    - name: "/embedded-product/delete"
      cname: Delete Embedded Product
    - name: "/embedded-product/put"
      cname: Edit Embedded Product
`
)
