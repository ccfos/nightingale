package engine

import (
	"reflect"
	"sync"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/aisummary"
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/callback"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// 记录 fake 处理器的真实执行序列，用于断言分段过滤只跳过该跳过的节点
var (
	stageTraceMu sync.Mutex
	stageTrace   []string
)

func resetStageTrace() {
	stageTraceMu.Lock()
	defer stageTraceMu.Unlock()
	stageTrace = nil
}

func appendStageTrace(name string) {
	stageTraceMu.Lock()
	defer stageTraceMu.Unlock()
	stageTrace = append(stageTrace, name)
}

func getStageTrace() []string {
	stageTraceMu.Lock()
	defer stageTraceMu.Unlock()
	return append([]string(nil), stageTrace...)
}

type traceTransformProcessor struct{}

func (p *traceTransformProcessor) Init(settings interface{}) (models.Processor, error) { return p, nil }
func (p *traceTransformProcessor) Process(c *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	appendStageTrace("transform")
	return wfCtx, "", nil
}

type traceActionProcessor struct{}

func (p *traceActionProcessor) Init(settings interface{}) (models.Processor, error) { return p, nil }
func (p *traceActionProcessor) Process(c *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	appendStageTrace("action")
	return wfCtx, "", nil
}
func (p *traceActionProcessor) Stage() models.ProcessorStage { return models.StageAction }

// traceBranchProcessor 固定选 1 号分支，验证动作段重放时路由与变换段一致
type traceBranchProcessor struct{}

func (p *traceBranchProcessor) Init(settings interface{}) (models.Processor, error) { return p, nil }
func (p *traceBranchProcessor) Process(c *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	return wfCtx, "", nil
}
func (p *traceBranchProcessor) ProcessWithBranch(c *ctx.Context, wfCtx *models.WorkflowContext) (*models.NodeOutput, error) {
	appendStageTrace("branch")
	idx := 1
	return &models.NodeOutput{WfCtx: wfCtx, BranchIndex: &idx}, nil
}

func init() {
	models.RegisterProcessor("test_trace_transform", &traceTransformProcessor{})
	models.RegisterProcessor("test_trace_action", &traceActionProcessor{})
	models.RegisterProcessor("test_trace_branch", &traceBranchProcessor{})
}

func execStage(t *testing.T, pipe *models.EventPipeline, stage models.ProcessorStage) (*models.AlertCurEvent, *models.WorkflowResult) {
	t.Helper()
	resetStageTrace()
	eng := NewWorkflowEngine(nil)
	// Mode 留空：测试不落执行记录
	event, result, err := eng.Execute(pipe, &models.AlertCurEvent{Hash: "hash-1"}, &models.WorkflowTriggerContext{Stage: stage})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	return event, result
}

func TestExecuteStageFiltersLinearPipeline(t *testing.T) {
	// 旧格式线性 pipeline：动作节点在前，验证被跳过后路由仍能走到后面的变换节点
	pipe := &models.EventPipeline{
		ID:   9101,
		Name: "stage-linear",
		ProcessorConfigs: []models.ProcessorConfig{
			{Typ: "test_trace_action"},
			{Typ: "test_trace_transform"},
		},
	}

	cases := []struct {
		stage models.ProcessorStage
		want  []string
	}{
		{models.StageTransform, []string{"transform"}},
		{models.StageAction, []string{"action"}},
		{"", []string{"action", "transform"}},
	}

	for _, c := range cases {
		event, result := execStage(t, pipe, c.stage)
		if event == nil {
			t.Fatalf("stage %q: event should not be nil", c.stage)
		}
		if got := getStageTrace(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("stage %q: executed %v, want %v", c.stage, got, c.want)
		}
		for _, nr := range result.NodeResults {
			if nr.Status == "failed" {
				t.Errorf("stage %q: node %s failed: %s", c.stage, nr.NodeName, nr.Error)
			}
		}
	}

	// 变换段里动作节点应标为 skipped 而不是缺失，保证执行记录里节点列表完整
	_, result := execStage(t, pipe, models.StageTransform)
	if len(result.NodeResults) != 2 || result.NodeResults[0].Status != "skipped" {
		t.Errorf("transform stage: action node should be skipped, got %+v", result.NodeResults)
	}
}

func TestExecuteStageBranchRoutingConsistent(t *testing.T) {
	// 分支节点两段都执行：0 号分支挂变换节点、1 号分支挂动作节点，分支固定选 1
	pipe := &models.EventPipeline{
		ID:   9102,
		Name: "stage-branch",
		Nodes: []models.WorkflowNode{
			{ID: "b", Name: "branch", Type: "test_trace_branch"},
			{ID: "t0", Name: "transform0", Type: "test_trace_transform"},
			{ID: "a1", Name: "action1", Type: "test_trace_action"},
		},
		Connections: models.Connections{
			"b": models.NodeConnections{Main: [][]models.ConnectionTarget{
				{{Node: "t0", Type: "main", Index: 0}},
				{{Node: "a1", Type: "main", Index: 0}},
			}},
		},
	}

	// 变换段：分支执行并选 1 号，动作节点被 stage 跳过，0 号分支的变换节点不应被执行
	execStage(t, pipe, models.StageTransform)
	if got := getStageTrace(); !reflect.DeepEqual(got, []string{"branch"}) {
		t.Errorf("transform stage executed %v, want [branch]", got)
	}

	// 动作段：分支重放选同一分支，动作节点执行
	execStage(t, pipe, models.StageAction)
	if got := getStageTrace(); !reflect.DeepEqual(got, []string{"branch", "action"}) {
		t.Errorf("action stage executed %v, want [branch action]", got)
	}
}

func TestBuiltinProcessorStageDeclarations(t *testing.T) {
	for name, p := range map[string]models.Processor{
		"callback":   &callback.CallbackConfig{},
		"ai_summary": &aisummary.AISummaryConfig{},
	} {
		if models.ProcessorRunsInStage(p, models.StageTransform) {
			t.Errorf("%s should not run in transform stage", name)
		}
		if !models.ProcessorRunsInStage(p, models.StageAction) {
			t.Errorf("%s should run in action stage", name)
		}
	}
}

func TestShouldPersistRecord(t *testing.T) {
	pipe := &models.EventPipeline{ID: 9201}
	okResult := &models.WorkflowResult{
		Status:      models.ExecutionStatusSuccess,
		NodeResults: []*models.NodeExecutionResult{{Status: "success"}},
	}
	skippedOnly := &models.WorkflowResult{
		Status:      models.ExecutionStatusSuccess,
		NodeResults: []*models.NodeExecutionResult{{Status: "skipped", SkippedByStage: true}},
	}
	nodeFailed := &models.WorkflowResult{
		Status:      models.ExecutionStatusSuccess,
		NodeResults: []*models.NodeExecutionResult{{Status: "failed"}},
	}

	// 默认策略：总是落库
	if !shouldPersistRecord(pipe, &models.WorkflowTriggerContext{}, okResult, false, "h") {
		t.Error("default policy should persist")
	}

	// 动作段：整条 pipeline 没有任何节点真正运行时不落空记录
	if shouldPersistRecord(pipe, &models.WorkflowTriggerContext{Stage: models.StageAction}, skippedOnly, false, "h") {
		t.Error("all-skipped stage run should not persist")
	}
	if !shouldPersistRecord(pipe, &models.WorkflowTriggerContext{Stage: models.StageAction}, okResult, false, "h") {
		t.Error("stage run with executed node should persist")
	}

	// 变换段（drop_or_fail）：正常执行不落，drop/节点失败才落
	transformCtx := &models.WorkflowTriggerContext{Stage: models.StageTransform, RecordPolicy: models.RecordOnDropOrFail}
	if shouldPersistRecord(pipe, transformCtx, okResult, false, "h-ok") {
		t.Error("drop_or_fail should not persist normal run")
	}
	if !shouldPersistRecord(pipe, transformCtx, okResult, true, "h-drop") {
		t.Error("drop_or_fail should persist dropped run")
	}
	// 同一 pipeline+事件哈希窗口内限流
	if shouldPersistRecord(pipe, transformCtx, okResult, true, "h-drop") {
		t.Error("dropped run should be throttled within window")
	}
	if !shouldPersistRecord(pipe, transformCtx, nodeFailed, false, "h-fail") {
		t.Error("drop_or_fail should persist failed run")
	}
}

func TestRecordThrottleAllow(t *testing.T) {
	if !recordThrottleAllow("throttle-key", 1000) {
		t.Error("first record should be allowed")
	}
	if recordThrottleAllow("throttle-key", 1000+recordThrottleWindow-1) {
		t.Error("record within window should be throttled")
	}
	if !recordThrottleAllow("throttle-key", 1000+recordThrottleWindow) {
		t.Error("record after window should be allowed")
	}
}

func TestMergeStageNodeResults(t *testing.T) {
	// 变换段结果：变换节点真实执行，动作节点是 stage 占位
	prior := []*models.NodeExecutionResult{
		{NodeID: "t", Status: "success", Message: "relabel | changes"},
		{NodeID: "a", Status: "skipped", SkippedByStage: true},
	}
	// 动作段结果：变换节点是 stage 占位，动作节点真实执行
	current := []*models.NodeExecutionResult{
		{NodeID: "t", Status: "skipped", SkippedByStage: true},
		{NodeID: "a", Status: "success", Message: "callback done"},
	}

	merged := mergeStageNodeResults(prior, current)
	if len(merged) != 2 {
		t.Fatalf("merged length = %d, want 2", len(merged))
	}
	if merged[0].NodeID != "t" || merged[0].Status != "success" || merged[0].Message != "relabel | changes" {
		t.Errorf("transform node should take prior real result, got %+v", merged[0])
	}
	if merged[1].NodeID != "a" || merged[1].Status != "success" || merged[1].Message != "callback done" {
		t.Errorf("action node should keep current real result, got %+v", merged[1])
	}

	// 前段没有对应真实结果的占位保持原样（如节点两段都被跳过）
	orphan := []*models.NodeExecutionResult{{NodeID: "x", Status: "skipped", SkippedByStage: true}}
	merged = mergeStageNodeResults(nil, orphan)
	if len(merged) != 1 || merged[0].Status != "skipped" {
		t.Errorf("orphan placeholder should be kept, got %+v", merged)
	}
}

func TestExecuteMergesPriorNodeResults(t *testing.T) {
	pipe := &models.EventPipeline{
		ID:   9103,
		Name: "stage-merge",
		ProcessorConfigs: []models.ProcessorConfig{
			{Typ: "test_trace_transform"},
			{Typ: "test_trace_action"},
		},
	}

	// 先跑变换段，拿到含真实变换结果的节点列表
	_, transformResult := execStage(t, pipe, models.StageTransform)

	// 动作段带上变换段结果，合并后两个节点都应是真实执行结果
	resetStageTrace()
	eng := NewWorkflowEngine(nil)
	_, result, err := eng.Execute(pipe, &models.AlertCurEvent{Hash: "hash-1"}, &models.WorkflowTriggerContext{
		Stage:            models.StageAction,
		PriorNodeResults: transformResult.NodeResults,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if len(result.NodeResults) != 2 {
		t.Fatalf("merged node results length = %d, want 2", len(result.NodeResults))
	}
	for _, nr := range result.NodeResults {
		if nr.Status != "success" || nr.SkippedByStage {
			t.Errorf("merged record should contain real results for both stages, got %+v", nr)
		}
	}
}
