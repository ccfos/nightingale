package tools

import (
	"strconv"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// 本文件实现写工具的缺参门：创建类
// 工具发现 group_id 缺失时，返回 input 类 ToolInterrupt——结构化业务组表单经
// router 渲染为 form_select（与 creation preflight 同一前端契约），用户提交后
// 带着补全的上下文重跑 agent。
//
// 这是 preflight 表单的执行层兜底：动词 fast-path 命中时 preflight 仍零 LLM 即时
// 弹表单；fast-path 漏网（意图绕弯的表达、通用路径下的创建）由本门接住，写操作
// 因此可以安全暴露给通用 agent，不再依赖"先分类对才有门"。

// resolveCreationGroupID 解析创建类工具的目标业务组：优先工具参数（模型显式
// 传入），回退 preflight/表单经 inputs 注入的 params["busi_group_id"]。
func resolveCreationGroupID(args map[string]interface{}, params map[string]string) int64 {
	if id := getArgInt64(args, "group_id"); id > 0 {
		return id
	}
	if s, ok := params["busi_group_id"]; ok {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// creationFormInterrupt 构造创建类工具的 input 中断：Form 为与 preflight 字节级
// 同构的 form_select 载荷，Prompt 为纯文本客户端（A2A）的兜底文案。
func creationFormInterrupt(deps *aiagent.ToolDeps, user *models.User, skillName string, required []string) *aiagent.ToolInterrupt {
	return &aiagent.ToolInterrupt{
		Kind:   aiagent.InterruptKindInput,
		Prompt: "创建前需要先确认目标业务组等必填信息。请在下方表单中选择后提交（也可以直接在回复里写明业务组名称或 ID），我会接着完成创建。",
		Form:   aiagent.BuildCreationForm(deps, user, skillName, required, aiagent.FormPreselect{}),
	}
}
