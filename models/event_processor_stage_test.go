package models

import (
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type stubPlainProcessor struct{}

func (p *stubPlainProcessor) Init(settings interface{}) (Processor, error) { return p, nil }
func (p *stubPlainProcessor) Process(c *ctx.Context, wfCtx *WorkflowContext) (*WorkflowContext, string, error) {
	return wfCtx, "", nil
}

type stubActionProcessor struct{ stubPlainProcessor }

func (p *stubActionProcessor) Stage() ProcessorStage { return StageAction }

type stubTransformProcessor struct{ stubPlainProcessor }

func (p *stubTransformProcessor) Stage() ProcessorStage { return StageTransform }

type stubBranchProcessor struct{ stubPlainProcessor }

func (p *stubBranchProcessor) ProcessWithBranch(c *ctx.Context, wfCtx *WorkflowContext) (*NodeOutput, error) {
	return &NodeOutput{WfCtx: wfCtx}, nil
}

func TestProcessorRunsInStage(t *testing.T) {
	plain := &stubPlainProcessor{}
	action := &stubActionProcessor{}
	transform := &stubTransformProcessor{}
	branch := &stubBranchProcessor{}

	cases := []struct {
		name  string
		p     Processor
		stage ProcessorStage
		want  bool
	}{
		// 完整执行（stage 为空）：所有处理器都执行
		{"plain full", plain, "", true},
		{"action full", action, "", true},
		{"branch full", branch, "", true},

		// 未声明 Stage 的处理器默认变换段，保持既有行为不被静默改变
		{"plain in transform", plain, StageTransform, true},
		{"plain in action", plain, StageAction, false},

		// 显式声明的按声明走
		{"action in transform", action, StageTransform, false},
		{"action in action", action, StageAction, true},
		{"transform in transform", transform, StageTransform, true},
		{"transform in action", transform, StageAction, false},

		// 分支处理器两段都执行，保证动作段路由与变换段一致
		{"branch in transform", branch, StageTransform, true},
		{"branch in action", branch, StageAction, true},
	}

	for _, c := range cases {
		if got := ProcessorRunsInStage(c.p, c.stage); got != c.want {
			t.Errorf("%s: ProcessorRunsInStage = %v, want %v", c.name, got, c.want)
		}
	}
}
