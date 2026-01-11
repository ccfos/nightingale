package models

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type Processor interface {
	Init(settings interface{}) (Processor, error) // 初始化配置
	Process(ctx *ctx.Context, wfCtx *WorkflowContext) (*WorkflowContext, string, error)
	// 处理器有三种情况：
	// 1. 处理成功，返回处理后的 WorkflowContext
	// 2. 处理成功，不需要返回处理后的上下文，只返回处理结果，将处理结果放到 string 中，比如 eventdrop callback 处理器
	// 3. 处理失败，返回错误，将错误放到 error 中
	// WorkflowContext 包含：Event（事件）、Env（环境变量/输入参数）、Metadata（执行元数据）
}

// BranchProcessor 分支处理器接口
// 用于 if、switch、foreach 等需要返回分支索引或特殊输出的处理器
type BranchProcessor interface {
	Processor
	// ProcessWithBranch 处理事件并返回 NodeOutput
	// NodeOutput 包含：处理后的上下文、消息、是否终止、分支索引
	ProcessWithBranch(ctx *ctx.Context, wfCtx *WorkflowContext) (*NodeOutput, error)
}

type NewProcessorFn func(settings interface{}) (Processor, error)

var processorRegister = map[string]NewProcessorFn{}

func RegisterProcessor(typ string, p Processor) {
	if _, found := processorRegister[typ]; found {
		return
	}
	processorRegister[typ] = p.Init
}

func GetProcessorByType(typ string, settings interface{}) (Processor, error) {
	typ = strings.TrimSpace(typ)
	fn, found := processorRegister[typ]
	if !found {
		return nil, fmt.Errorf("processor type %s not found", typ)
	}

	processor, err := fn(settings)
	if err != nil {
		return nil, err
	}

	return processor, nil
}
