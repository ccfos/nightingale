package config

import "github.com/didi/nightingale/v5/pkg/i18n"

var (
	dict = map[string]string{
		"Login fail, check your username and password":               "登录失败，请检查您的用户名和密码",
		"Internal server error, try again later please":              "系统内部错误，请稍后再试",
		"Each user has at most two tokens":                           "每个用户至多创建两个密钥",
		"No such token":                                              "密钥不存在",
		"Username is blank":                                          "用户名不能为空",
		"Username has invalid characters":                            "用户名含有非法字符",
		"Nickname has invalid characters":                            "用户昵称含有非法字符",
		"Phone invalid":                                              "手机号格式有误",
		"Email invalid":                                              "邮箱格式有误",
		"Incorrect old password":                                     "旧密码错误",
		"Username %s already exists":                                 "用户名(%s)已存在",
		"No such user":                                               "用户不存在",
		"UserGroup %s already exists":                                "用户组(%s)已存在",
		"Group name has invalid characters":                          "分组名称含有非法字符",
		"Group note has invalid characters":                          "分组备注含有非法字符",
		"No such user group":                                         "用户组不存在",
		"Classpath path has invalid characters":                      "机器分组路径含有非法字符",
		"Classpath note has invalid characters":                      "机器分组路径备注含有非法字符",
		"There are still resources under the classpath":              "机器分组路径下仍然挂有资源",
		"There are still collect rules under the classpath":          "机器分组路径下仍然存在采集策略",
		"No such classpath":                                          "机器分组路径不存在",
		"Classpath %s already exists":                                "机器分组路径(%s)已存在",
		"Preset classpath %s cannot delete":                          "内置机器分组(%s)不允许删除",
		"No such mute config":                                        "此屏蔽配置不存在",
		"DashboardGroup name has invalid characters":                 "大盘分组名称含有非法字符",
		"DashboardGroup name is blank":                               "大盘分组名称为空",
		"DashboardGroup %s already exists":                           "大盘分组(%s)已存在",
		"No such dashboard group":                                    "大盘分组不存在",
		"Dashboard name has invalid characters":                      "大盘名称含有非法字符",
		"Dashboard %s already exists":                                "监控大盘(%s)已存在",
		"ChartGroup name has invalid characters":                     "图表分组名称含有非法字符",
		"No such dashboard":                                          "监控大盘不存在",
		"No such chart group":                                        "图表分组不存在",
		"No such chart":                                              "图表不存在",
		"There are still dashboards under the group":                 "分组下面仍然存在监控大盘，请先从组内移出",
		"AlertRuleGroup name has invalid characters":                 "告警规则分组含有非法字符",
		"AlertRuleGroup %s already exists":                           "告警规则分组(%s)已存在",
		"There are still alert rules under the group":                "分组下面仍然存在告警规则",
		"AlertRule name has invalid characters":                      "告警规则含有非法字符",
		"No such alert rule":                                         "告警规则不存在",
		"No such alert rule group":                                   "告警规则分组不存在",
		"No such alert event":                                        "告警事件不存在",
		"No such collect rule":                                       "采集规则不存在",
		"Decoded metric description empty":                           "导入的指标释义列表为空",
		"User disabled":                                              "用户已被禁用",
		"Tags(%s) invalid":                                           "标签(%s)格式不合法",
		"Resource filter(Func:%s)'s param invalid":                   "资源过滤条件(函数：%s)参数不合法(为空或包含空格都不合法)",
		"Tags filter(Func:%s)'s param invalid":                       "标签过滤条件(函数：%s)参数不合法(为空或包含空格都不合法)",
		"Regexp: %s cannot be compiled":                              "正则表达式(%s)不合法，无法编译",
		"AppendTags(%s) invalid":                                     "附件标签(%s)格式不合法",
		"Regexp[%s] matching failed":                                 "正则表达式[%s]匹配失败",
		"Regexp[%s] matched, but cannot get substring()":             "主正则[%s]匹配成功，但无法匹配到子串",
		"TagKey or TagValue contains illegal characters[:,/=\r\n\t]": "标签KEY或者标签值包含非法字符串[:,/=\r\n\t]",
	}
	langDict = map[string]map[string]string{
		"zh": dict,
	}
)

func init() {
	i18n.DictRegister(langDict)
}
