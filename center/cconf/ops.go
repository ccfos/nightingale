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
    - name: /notification-rules
      cname: Notification Rule - View
    - name: /notification-rules/add
      cname: Notification Rule - Add
    - name: /notification-rules/put
      cname: Notification Rule - Modify
    - name: /notification-rules/del
      cname: Notification Rule - Delete
    - name: /notification-channels
      cname: Media Type - View
    - name: /notification-channels/add
      cname: Media Type - Add
    - name: /notification-channels/put
      cname: Media Type - Modify
    - name: /notification-channels/del
      cname: Media Type - Delete
    - name: /notification-templates
      cname: Message Template - View
    - name: /notification-templates/add
      cname: Message Template - Add
    - name: /notification-templates/put
      cname: Message Template - Modify
    - name: /notification-templates/del
      cname: Message Template - Delete
    - name: /event-pipelines
      cname: Event Pipeline - View
    - name: /event-pipelines/add
      cname: Event Pipeline - Add
    - name: /event-pipelines/put
      cname: Event Pipeline - Modify
    - name: /event-pipelines/del
      cname: Event Pipeline - Delete
    - name: /help/notification-settings # 用于控制老版本的通知设置菜单是否展示
      cname: Notification Settings - View
    - name: /help/notification-tpls # 用于控制老版本的通知模板菜单是否展示
      cname: Notification Templates - View

- name: Integrations
  cname: Integrations
  ops:
    - name: /datasources # 用于控制能否看到数据源列表页面的菜单。只有 Admin 才能修改、删除数据源
      cname: Data Source - View
    - name: /components
      cname: Component - View
    - name: /components/add
      cname: Component - Add
    - name: /components/put
      cname: Component - Modify
    - name: /components/del
      cname: Component - Delete
    - name: /embedded-products
      cname: Embedded Product - View
    - name: /embedded-product/add
      cname: Embedded Product - Add
    - name: /embedded-product/put
      cname: Embedded Product - Modify
    - name: /embedded-product/delete
      cname: Embedded Product - Delete

- name: Organization
  cname: Organization
  ops:
    - name: /users
      cname: User - View
    - name: /users/add
      cname: User - Add
    - name: /users/put
      cname: User - Modify
    - name: /users/del
      cname: User - Delete
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
    - name: /roles/add
      cname: Role - Add
    - name: /roles/put
      cname: Role - Modify
    - name: /roles/del
      cname: Role - Delete

- name: System Settings
  cname: System Settings
  ops:
    - name: /system/site-settings # 仅用于控制能否展示菜单，只有 Admin 才能修改、删除
      cname: View Site Settings
    - name: /system/variable-settings
      cname: View Variable Settings
    - name: /system/sso-settings
      cname: View SSO Settings
    - name: /system/alerting-engines
      cname: View Alerting Engines
    - name: /system/version
      cname: View Product Version

`
)
