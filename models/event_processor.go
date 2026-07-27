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

// ProcessorStage 处理器执行段。告警规则上的 pipeline 分两段执行：
// 变换段在评估阶段、屏蔽检测之前运行（屏蔽匹配依赖处理后的标签），每个评估周期都会执行；
// 动作段只在事件真正产生（落库/通知）时运行，避免外部副作用按评估频率空跑。
type ProcessorStage string

const (
	// StageTransform 变换段：纯内存修改事件内容的处理器，如 relabel、event_update、event_drop
	StageTransform ProcessorStage = "transform"
	// StageAction 动作段：有外部副作用或高成本的处理器，如 callback、ai_summary
	StageAction ProcessorStage = "action"
)

// StagedProcessor 可选接口：处理器声明自己所属的执行段。
// 未实现该接口的处理器默认归为变换段，保持既有行为（每周期、屏蔽检测前执行）不被静默改变。
type StagedProcessor interface {
	Stage() ProcessorStage
}

// ProcessorRunsInStage 判断处理器在指定执行段是否需要执行。
// stage 为空表示完整执行（手动测试、notify_rule 等场景）；
// 分支处理器（if/switch）两段都执行，保证动作段重放时路由与变换段一致——
// 分支条件读的是事件字段，动作段拿到的已是变换后的事件，重算结果相同。
func ProcessorRunsInStage(p Processor, stage ProcessorStage) bool {
	if stage == "" {
		return true
	}

	if _, ok := p.(BranchProcessor); ok {
		return true
	}

	if sp, ok := p.(StagedProcessor); ok {
		return sp.Stage() == stage
	}

	return stage == StageTransform
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
