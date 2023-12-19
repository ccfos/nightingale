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
	Name  string   `yaml:"name" json:"name"`
	Cname string   `yaml:"cname" json:"cname"`
	Ops   []string `yaml:"ops" json:"ops"`
}

func LoadOpsYaml(configDir string, opsYamlFile string) error {
	fp := opsYamlFile
	if fp == "" {
		fp = path.Join(configDir, "ops.yaml")
	}
	if !file.IsExist(fp) {
		return nil
	}
	return file.ReadYaml(fp, &Operations)
}

func GetAllOps(ops []Ops) []string {
	var ret []string
	for _, op := range ops {
		ret = append(ret, op.Ops...)
	}
	return ret
}

func MergeOperationConf() error {
	opsBuiltIn := Operation{}
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
  cname: 仪表盘
  ops:
    - "/dashboards"
    - "/dashboards/add"
    - "/dashboards/put"
    - "/dashboards/del"
    - "/dashboards-built-in"

- name: alert
  cname: 告警规则
  ops:
    - "/alert-rules"
    - "/alert-rules/add"
    - "/alert-rules/put"
    - "/alert-rules/del"
    - "/alert-rules-built-in"
- name: alert-mutes
  cname: 告警静默管理
  ops:
    - "/alert-mutes"
    - "/alert-mutes/add"
    - "/alert-mutes/put"
    - "/alert-mutes/del"
  
- name: alert-subscribes
  cname: 告警订阅管理
  ops:
    - "/alert-subscribes"
    - "/alert-subscribes/add"
    - "/alert-subscribes/put"
    - "/alert-subscribes/del"

- name: alert-events  
  cname: 告警事件管理
  ops:
    - "/alert-cur-events"
    - "/alert-cur-events/del"
    - "/alert-his-events" 

- name: recording-rules
  cname: 记录规则管理
  ops:
    - "/recording-rules"
    - "/recording-rules/add"
    - "/recording-rules/put"
    - "/recording-rules/del"

- name: metric
  cname: 时序指标
  ops:
  - "/metric/explorer"
  - "/object/explorer"

- name: log
  cname: 日志分析
  ops:
  - "/log/explorer"
  - "/log/index-patterns"

- name: targets
  cname: 基础设施
  ops:
    - "/targets"
    - "/targets/add"
    - "/targets/put"
    - "/targets/del"
    - "/targets/bind"

- name: job
  cname: 任务管理
  ops:
    - "/job-tpls"
    - "/job-tpls/add"
    - "/job-tpls/put"
    - "/job-tpls/del"
    - "/job-tasks"
    - "/job-tasks/add"
    - "/job-tasks/put"
    - "/ibex-settings"

- name: user
  cname: 用户管理
  ops:
  - "/users"
  - "/user-groups"
  - "/user-groups/add"
  - "/user-groups/put"
  - "/user-groups/del"

- name: permissions
  cname: 权限管理
  ops:
  - "/permissions"

- name: busi-groups
  cname: 业务分组管理
  ops:
  - "/busi-groups"
  - "/busi-groups/add"
  - "/busi-groups/put"
  - "/busi-groups/del"

- name: system
  cname: 系统信息
  ops:
  - "/help/variable-configs"
  - "/help/version"
  - "/help/servers"
  - "/help/source"
  - "/help/sso"
  - "/help/notification-tpls"
  - "/help/notification-settings"
  - "/help/migrate"
  - "/site-settings"
`
)
