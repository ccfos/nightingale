package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/chat"
	"github.com/ccfos/nightingale/v6/models"
)

// TestResolveActionKey：路由收缩后的确定性 action 解析，全部分支零 LLM。
// 前端 action.key 通道已废弃（fe 发送前一律剥掉 key），不存在 front 分支。
func TestResolveActionKey(t *testing.T) {
	formRoute := &models.ConversationRoute{ActionKey: "creation", AwaitingForm: true}
	plainRoute := &models.ConversationRoute{ActionKey: "creation"}

	cases := []struct {
		name       string
		content    string
		paramCount int
		prevRoute  *models.ConversationRoute
		wantKey    string
		wantMethod string
	}{
		{"创建动词 fast-path", "创建一条 CPU 告警规则", 0, nil, "creation", "fast"},
		{"表单提交继承上轮 action", "业务组：123", 1, formRoute, "creation", "form"},
		{"无 param 的跟进不继承（防话题切换误粘）", "现在有哪些告警", 0, formRoute, "general_chat", "default"},
		{"上轮非表单收尾不继承", "业务组：123", 1, plainRoute, "general_chat", "default"},
		{"开放输入默认通用路径", "怎么部署 categraf", 0, nil, "general_chat", "default"},
		{"查询动词不触发 fast-path", "查看已创建的告警规则", 0, nil, "general_chat", "default"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, method := resolveActionKey(c.content, c.paramCount, c.prevRoute)
			if key != c.wantKey || method != c.wantMethod {
				t.Fatalf("resolveActionKey() = (%s, %s), want (%s, %s)", key, method, c.wantKey, c.wantMethod)
			}
		})
	}
}

// TestGeneralChatFormSubmissionBackfill：general_chat 表单回环回归。
// 通用路径上创建类工具触发缺参门（tools/form_gate.go）弹表单后，提交轮被
// resolveActionKey 以 "form" 继承回 general_chat——表单值必须经 router 默认的
// Context→inputs 转发抵达工具层 params（缺参门的确定性回填通道
// params["busi_group_id"]），不能只存在于提示词文本里——否则模型不复写
// group_id 时缺参门会再次弹出同一张表单，形成死循环。
func TestGeneralChatFormSubmissionBackfill(t *testing.T) {
	// 上轮：general_chat 路径缺参门以 form_select 收尾
	prevRoute := &models.ConversationRoute{ActionKey: "general_chat", AwaitingForm: true}

	// 本轮：表单提交（content 不含创建动词、带 action.param；JSON 数字解码为 float64）
	param := map[string]interface{}{"busi_group_id": float64(2), "datasource_id": float64(5)}
	actionKey, method := resolveActionKey("好的", len(param), prevRoute)
	if actionKey != "general_chat" || method != "form" {
		t.Fatalf("resolveActionKey() = (%s, %s), want (general_chat, form)", actionKey, method)
	}

	if _, ok := chat.Lookup(actionKey); !ok {
		t.Fatalf("chat.Lookup(%s) miss", actionKey)
	}

	// 与 processAssistantMessage 一致：action.param 原样合并进 Context
	chatReq := &chat.AIChatRequest{ActionKey: actionKey, UserInput: "好的", Context: map[string]interface{}{}}
	for k, v := range param {
		chatReq.Context[k] = v
	}

	inputs := buildAgentInputs(chatReq, 99, "chat-1", 2)
	if inputs["busi_group_id"] != "2" {
		t.Fatalf("busi_group_id 未抵达工具层 params（缺参门将再次弹表单）: %v", inputs)
	}
	if inputs["datasource_id"] != "5" {
		t.Fatalf("datasource_id 未抵达工具层 params: %v", inputs)
	}
	for k, want := range map[string]string{"user_input": "好的", "user_id": "99", "chat_id": "chat-1", "seq_id": "2"} {
		if inputs[k] != want {
			t.Fatalf("inputs[%s] = %q, want %q", k, inputs[k], want)
		}
	}
}
