package cconf

import (
	"fmt"
	"path"

	"github.com/toolkits/pkg/file"
	"gopkg.in/yaml.v2"
)

var Operations = Operation{}

// CnameToName 用于读接口转换成易读形式
var CnameToName = make(map[string]string)

// NameToCname 用于写接口转换成 url 形式
var NameToCname = make(map[string]string)

type Operation struct {
	Ops []Ops `yaml:"ops"`
}

type Ops struct {
	Name  string   `json:"name"`
	Cname string   `json:"cname"`
	Ops   []string `json:"ops"`
}

// OperationHelper 用于解析内置的信息
type OperationHelper struct {
	Ops []OpsHelper `yaml:"ops"`
}

type OpsHelper struct {
	Name  string     `yaml:"name"`
	Cname string     `yaml:"cname"`
	Ops   []SingleOp `yaml:"ops"`
}

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

func GetAllOps(ops []Ops) []string {
	var ret []string
	for _, op := range ops {
		ret = append(ret, op.Ops...)
	}
	return ret
}

func MergeOperationConf() error {
	var operationHelper OperationHelper
	err := yaml.Unmarshal([]byte(builtInOps), &operationHelper)
	if err != nil {
		return fmt.Errorf("cannot parse builtInOps: %s", err.Error())
	}

	opsBuiltIn := Operation{}
	for i := range operationHelper.Ops {
		op := Ops{
			Name:  operationHelper.Ops[i].Name,
			Cname: operationHelper.Ops[i].Cname,
		}
		for j := range operationHelper.Ops[i].Ops {
			op.Ops = append(op.Ops, operationHelper.Ops[i].Ops[j].Cname)
			CnameToName[operationHelper.Ops[i].Ops[j].Cname] = operationHelper.Ops[i].Ops[j].Name
			NameToCname[operationHelper.Ops[i].Ops[j].Name] = operationHelper.Ops[i].Ops[j].Cname
		}
		opsBuiltIn.Ops = append(opsBuiltIn.Ops, op)
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
    - name: 读取仪表盘信息
      cname: "/dashboards"
    - name: 添加仪表盘
      cname: "/dashboards/add"
    - name: 修改仪表盘
      cname: "/dashboards/put"
    - name: 删除仪表盘
      cname: "/dashboards/del"
    - name: 修改嵌入仪表盘
      cname: "/embedded-dashboards/put"
    - name: 查看嵌入仪表盘
      cname: "/embedded-dashboards"
    - name: 查看公开仪表盘
      cname: "/public-dashboards"

- name: alert
  cname: 告警规则
  ops:
    - name: 查看告警规则
      cname: "/alert-rules"
    - name: 添加告警规则
      cname: "/alert-rules/add"
    - name: 修改告警规则
      cname: "/alert-rules/put"
    - name: 删除告警规则
      cname: "/alert-rules/del"

- name: alert-mutes
  cname: 告警静默管理
  ops:
    - name: 查看告警静默
      cname: "/alert-mutes"
    - name: 添加告警静默
      cname: "/alert-mutes/add"
    - name: 修改告警静默
      cname: "/alert-mutes/put"
    - name: 删除告警静默
      cname: "/alert-mutes/del"
  
- name: alert-subscribes
  cname: 告警订阅管理
  ops:
    - name: 查看告警订阅
      cname: "/alert-subscribes"
    - name: 添加告警订阅
      cname: "/alert-subscribes/add"
    - name: 修改告警订阅
      cname: "/alert-subscribes/put"
    - name: 删除告警订阅
      cname: "/alert-subscribes/del"

- name: alert-events  
  cname: 告警事件管理
  ops:
    - name: 查看当前告警
      cname: "/alert-cur-events"
    - name: 删除当前告警
      cname: "/alert-cur-events/del"
    - name: 查看历史告警
      cname: "/alert-his-events"

- name: recording-rules
  cname: 记录规则管理
  ops:
    - name: 查看记录规则
      cname: "/recording-rules"
    - name: 添加记录规则
      cname: "/recording-rules/add"
    - name: 修改记录规则
      cname: "/recording-rules/put"
    - name: 删除记录规则
      cname: "/recording-rules/del"

- name: metric
  cname: 时序指标
  ops:
    - name: 查看指标数据
      cname: "/metric/explorer"
    - name: 查看对象数据
      cname: "/object/explorer"

- name: log
  cname: 日志分析
  ops:
    - name: 查看日志
      cname: "/log/explorer"
    - name: 查看索引模式
      cname: "/log/index-patterns"

- name: targets
  cname: 基础设施
  ops:
    - name: 查看对象
      cname: "/targets"
    - name: 添加对象
      cname: "/targets/add"
    - name: 修改对象
      cname: "/targets/put"
    - name: 删除对象
      cname: "/targets/del"
    - name: 绑定对象
      cname: "/targets/bind"

- name: job
  cname: 任务管理
  ops:
    - name: 查看任务模板
      cname: "/job-tpls"
    - name: 添加任务模板
      cname: "/job-tpls/add"
    - name: 修改任务模板
      cname: "/job-tpls/put"
    - name: 删除任务模板
      cname: "/job-tpls/del"
    - name: 查看任务实例
      cname: "/job-tasks"
    - name: 添加任务实例
      cname: "/job-tasks/add"
    - name: 修改任务实例
      cname: "/job-tasks/put"
    - name: 查看任务设置
      cname: "/ibex-settings"

- name: user
  cname: 用户管理
  ops:
    - name: 查看用户列表
      cname: "/users"
    - name: 查看用户组
      cname: "/user-groups"
    - name: 添加用户组
      cname: "/user-groups/add"
    - name: 修改用户组
      cname: "/user-groups/put"
    - name: 删除用户组
      cname: "/user-groups/del"

- name: permissions
  cname: 权限管理
  ops:
    - name: 查看权限配置
      cname: "/permissions"

- name: busi-groups
  cname: 业务分组管理
  ops:
    - name: 查看业务分组
      cname: "/busi-groups"
    - name: 添加业务分组
      cname: "/busi-groups/add"
    - name: 修改业务分组
      cname: "/busi-groups/put"
    - name: 删除业务分组
      cname: "/busi-groups/del"

- name: builtin-metrics
  cname: 指标视图
  ops:
    - name: 查看内置指标
      cname: "/metrics-built-in"
    - name: 添加内置指标
      cname: "/builtin-metrics/add"
    - name: 修改内置指标
      cname: "/builtin-metrics/put"
    - name: 删除内置指标
      cname: "/builtin-metrics/del"

- name: built-in-components
  cname: 模版中心
  ops:
    - name: 查看内置组件
      cname: "/built-in-components"
    - name: 添加内置组件
      cname: "/built-in-components/add"
    - name: 修改内置组件
      cname: "/built-in-components/put"
    - name: 删除内置组件
      cname: "/built-in-components/del"

- name: system
  cname: 系统信息
  ops:
    - name: 查看变量配置
      cname: "/help/variable-configs"
    - name: 查看版本信息
      cname: "/help/version"
    - name: 查看服务器信息
      cname: "/help/servers"
    - name: 查看数据源配置
      cname: "/help/source"
    - name: 查看SSO配置
      cname: "/help/sso"
    - name: 查看通知模板
      cname: "/help/notification-tpls"
    - name: 查看通知设置
      cname: "/help/notification-settings"
    - name: 查看迁移配置
      cname: "/help/migrate"
    - name: 查看站点设置
      cname: "/site-settings"
`
)
