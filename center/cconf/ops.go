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

// SingleOp Name 为 op 名称；Cname 为展示名称，默认中文
type SingleOp struct {
	Name  string `yaml:"name"`
	Cname string `yaml:"cname"`
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
  cname: 仪表盘
  ops:
    - name: "/dashboards"
      cname: 读取仪表盘信息
    - name: "/dashboards/add"
      cname: 添加仪表盘
    - name: "/dashboards/put"
      cname: 修改仪表盘
    - name: "/dashboards/del"
      cname: 删除仪表盘
    - name: "/embedded-dashboards/put"
      cname: 修改嵌入仪表盘
    - name: "/embedded-dashboards"
      cname: 查看嵌入仪表盘
    - name: "/public-dashboards"
      cname: 查看公开仪表盘

- name: alert
  cname: 告警规则
  ops:
    - name: "/alert-rules"
      cname: 查看告警规则
    - name: "/alert-rules/add"
      cname: 添加告警规则
    - name: "/alert-rules/put"
      cname: 修改告警规则
    - name: "/alert-rules/del"
      cname: 删除告警规则

- name: alert-mutes
  cname: 告警静默管理
  ops:
    - name: "/alert-mutes"
      cname: 查看告警静默
    - name: "/alert-mutes/add"
      cname: 添加告警静默
    - name: "/alert-mutes/put"
      cname: 修改告警静默
    - name: "/alert-mutes/del"
      cname: 删除告警静默
  
- name: alert-subscribes
  cname: 告警订阅管理
  ops:
    - name: "/alert-subscribes"
      cname: 查看告警订阅
    - name: "/alert-subscribes/add"
      cname: 添加告警订阅
    - name: "/alert-subscribes/put"
      cname: 修改告警订阅
    - name: "/alert-subscribes/del"
      cname: 删除告警订阅

- name: alert-events  
  cname: 告警事件管理
  ops:
    - name: "/alert-cur-events"
      cname: 查看当前告警
    - name: "/alert-cur-events/del"
      cname: 删除当前告警
    - name: "/alert-his-events"
      cname: 查看历史告警

- name: recording-rules
  cname: 记录规则管理
  ops:
    - name: "/recording-rules"
      cname: 查看记录规则
    - name: "/recording-rules/add"
      cname: 添加记录规则
    - name: "/recording-rules/put"
      cname: 修改记录规则
    - name: "/recording-rules/del"
      cname: 删除记录规则

- name: metric
  cname: 时序指标
  ops:
    - name: "/metric/explorer"
      cname: 查看指标数据
    - name: "/object/explorer"
      cname: 查看对象数据

- name: log
  cname: 日志分析
  ops:
    - name: "/log/explorer"
      cname: 查看日志
    - name: "/log/index-patterns"
      cname: 查看索引模式

- name: targets
  cname: 基础设施
  ops:
    - name: "/targets"
      cname: 查看对象
    - name: "/targets/add"
      cname: 添加对象
    - name: "/targets/put"
      cname: 修改对象
    - name: "/targets/del"
      cname: 删除对象
    - name: "/targets/bind"
      cname: 绑定对象

- name: job
  cname: 任务管理
  ops:
    - name: "/job-tpls"
      cname: 查看任务模板
    - name: "/job-tpls/add"
      cname: 添加任务模板
    - name: "/job-tpls/put"
      cname: 修改任务模板
    - name: "/job-tpls/del"
      cname: 删除任务模板
    - name: "/job-tasks"
      cname: 查看任务实例
    - name: "/job-tasks/add"
      cname: 添加任务实例
    - name: "/job-tasks/put"
      cname: 修改任务实例
    - name: "/ibex-settings"
      cname: 查看任务设置

- name: user
  cname: 用户管理
  ops:
    - name: "/users"
      cname: 查看用户列表
    - name: "/user-groups"
      cname: 查看用户组
    - name: "/user-groups/add"
      cname: 添加用户组
    - name: "/user-groups/put"
      cname: 修改用户组
    - name: "/user-groups/del"
      cname: 删除用户组

- name: permissions
  cname: 权限管理
  ops:
    - name: "/permissions"
      cname: 查看权限配置

- name: busi-groups
  cname: 业务分组管理
  ops:
    - name: "/busi-groups"
      cname: 查看业务分组
    - name: "/busi-groups/add"
      cname: 添加业务分组
    - name: "/busi-groups/put"
      cname: 修改业务分组
    - name: "/busi-groups/del"
      cname: 删除业务分组

- name: builtin-metrics
  cname: 指标视图
  ops:
    - name: "/metrics-built-in"
      cname: 查看内置指标
    - name: "/builtin-metrics/add"
      cname: 添加内置指标
    - name: "/builtin-metrics/put"
      cname: 修改内置指标
    - name: "/builtin-metrics/del"
      cname: 删除内置指标

- name: built-in-components
  cname: 模版中心
  ops:
    - name: "/built-in-components"
      cname: 查看内置组件
    - name: "/built-in-components/add"
      cname: 添加内置组件
    - name: "/built-in-components/put"
      cname: 修改内置组件
    - name: "/built-in-components/del"
      cname: 删除内置组件

- name: system
  cname: 系统信息
  ops:
    - name: "/help/variable-configs"
      cname: 查看变量配置
    - name: "/help/version"
      cname: 查看版本信息
    - name: "/help/servers"
      cname: 查看服务器信息
    - name: "/help/source"
      cname: 查看数据源配置
    - name: "/help/sso"
      cname: 查看SSO配置
    - name: "/help/notification-tpls"
      cname: 查看通知模板
    - name: "/help/notification-settings"
      cname: 查看通知设置
    - name: "/help/migrate"
      cname: 查看迁移配置
    - name: "/site-settings"
      cname: 查看站点设置
`
)
