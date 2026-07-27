package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

type WorkflowEngine struct {
	ctx *ctx.Context
}

func NewWorkflowEngine(c *ctx.Context) *WorkflowEngine {
	return &WorkflowEngine{ctx: c}
}

func (e *WorkflowEngine) Execute(pipeline *models.EventPipeline, event *models.AlertCurEvent, triggerCtx *models.WorkflowTriggerContext) (*models.AlertCurEvent, *models.WorkflowResult, error) {
	startTime := time.Now()

	wfCtx := e.initWorkflowContext(pipeline, event, triggerCtx)

	// 事件可能被 drop（wfCtx.Event 置 nil），落库限流的 key 要用执行前的哈希
	eventHash := ""
	if event != nil {
		eventHash = event.Hash
	}

	nodes := pipeline.GetWorkflowNodes()
	connections := pipeline.GetWorkflowConnections()

	if len(nodes) == 0 {
		return event, &models.WorkflowResult{
			Event:   event,
			Status:  models.ExecutionStatusSuccess,
			Message: "no nodes to execute",
		}, nil
	}

	nodeMap := make(map[string]*models.WorkflowNode)
	for i := range nodes {
		if nodes[i].RetryInterval == 0 {
			nodes[i].RetryInterval = 1
		}

		if nodes[i].MaxRetries == 0 {
			nodes[i].MaxRetries = 1
		}

		nodeMap[nodes[i].ID] = &nodes[i]
	}

	result := e.executeDAG(nodeMap, connections, wfCtx)
	result.Event = wfCtx.Event

	// 动作段执行时把变换段的真实节点结果合并进来：
	// 本段被 stage 跳过的占位替换为变换段的实际执行结果，形成一条完整记录
	if triggerCtx != nil && len(triggerCtx.PriorNodeResults) > 0 {
		result.NodeResults = mergeStageNodeResults(triggerCtx.PriorNodeResults, result.NodeResults)
	}

	duration := time.Since(startTime).Milliseconds()

	if triggerCtx != nil && triggerCtx.Mode != "" && shouldPersistRecord(pipeline, triggerCtx, result, wfCtx.Event == nil, eventHash) {
		e.saveExecutionRecord(pipeline, wfCtx, result, triggerCtx, startTime.Unix(), duration)
	}

	return wfCtx.Event, result, nil
}

// shouldPersistRecord 判断本次执行是否落执行记录。
// 变换段传 RecordOnDropOrFail：正常执行不落（每评估周期都执行，落库会使记录表按评估频率增长，
// 其真实结果暂存事件上、事件产生时随动作段合并落库），仅事件被丢弃或有节点失败时落一条留痕
// ——这是事件不会产生、合并记录无从落的场景，drop 动作需要在执行记录里可见；
// drop/失败本身也按评估周期重复发生，需按 pipeline+事件哈希限流。
// 动作段在事件真正产生时落合并后的完整记录，但整条 pipeline 没有任何节点真正运行时不落空记录。
func shouldPersistRecord(pipeline *models.EventPipeline, triggerCtx *models.WorkflowTriggerContext, result *models.WorkflowResult, dropped bool, eventHash string) bool {
	if triggerCtx.RecordPolicy == models.RecordOnDropOrFail {
		failed := result.Status == models.ExecutionStatusFailed
		for _, nr := range result.NodeResults {
			if failed {
				break
			}
			if nr.Status == "failed" {
				failed = true
			}
		}

		if !dropped && !failed {
			return false
		}

		return recordThrottleAllow(fmt.Sprintf("%d|%s|%s", pipeline.ID, triggerCtx.Stage, eventHash), time.Now().Unix())
	}

	if triggerCtx.Stage != "" {
		for _, nr := range result.NodeResults {
			if nr.Status != "skipped" {
				return true
			}
		}
		return false
	}

	return true
}

// recordThrottleWindow drop/失败记录的落库限流窗口（秒）：同一 pipeline+事件哈希窗口内只落一条
const recordThrottleWindow = int64(3600)

var (
	recordThrottleMu   sync.Mutex
	recordThrottleSeen = map[string]int64{}
)

func recordThrottleAllow(key string, now int64) bool {
	recordThrottleMu.Lock()
	defer recordThrottleMu.Unlock()

	// 机会式清理过期 key，防止事件哈希长期累积
	if len(recordThrottleSeen) > 8192 {
		for k, ts := range recordThrottleSeen {
			if now-ts >= recordThrottleWindow {
				delete(recordThrottleSeen, k)
			}
		}
	}

	if ts, ok := recordThrottleSeen[key]; ok && now-ts < recordThrottleWindow {
		return false
	}

	recordThrottleSeen[key] = now
	return true
}

// mergeStageNodeResults 把前一段（变换段）的真实节点结果合并进本段结果：
// 本段中因分段执行被跳过的节点，用前一段同 ID 节点的真实结果替换；
// 本段真正执行过的节点（动作节点、分支节点重放）保留本段结果。
func mergeStageNodeResults(prior, current []*models.NodeExecutionResult) []*models.NodeExecutionResult {
	priorByID := make(map[string]*models.NodeExecutionResult, len(prior))
	for _, nr := range prior {
		if nr != nil && !nr.SkippedByStage {
			priorByID[nr.NodeID] = nr
		}
	}

	merged := make([]*models.NodeExecutionResult, 0, len(current))
	for _, nr := range current {
		if nr != nil && nr.SkippedByStage {
			if p, ok := priorByID[nr.NodeID]; ok {
				merged = append(merged, p)
				continue
			}
		}
		merged = append(merged, nr)
	}
	return merged
}

func (e *WorkflowEngine) initWorkflowContext(pipeline *models.EventPipeline, event *models.AlertCurEvent, triggerCtx *models.WorkflowTriggerContext) *models.WorkflowContext {
	// 合并输入参数
	inputs := pipeline.GetInputsMap()
	if triggerCtx != nil && triggerCtx.InputsOverrides != nil {
		for k, v := range triggerCtx.InputsOverrides {
			inputs[k] = v
		}
	}

	metadata := map[string]string{
		"start_time":  fmt.Sprintf("%d", time.Now().Unix()),
		"pipeline_id": fmt.Sprintf("%d", pipeline.ID),
	}

	// 是否启用流式输出
	stream := false
	var stage models.ProcessorStage
	if triggerCtx != nil {
		metadata["request_id"] = triggerCtx.RequestID
		metadata["trigger_mode"] = triggerCtx.Mode
		metadata["trigger_by"] = triggerCtx.TriggerBy
		stream = triggerCtx.Stream
		stage = triggerCtx.Stage
	}

	return &models.WorkflowContext{
		Event:    event,
		Inputs:   inputs,
		Vars:     make(map[string]interface{}), // 初始化空的 Vars，供节点间传递数据
		Metadata: metadata,
		Stream:   stream,
		Stage:    stage,
	}
}

// executeDAG 使用 Kahn 算法执行 DAG
func (e *WorkflowEngine) executeDAG(nodeMap map[string]*models.WorkflowNode, connections models.Connections, wfCtx *models.WorkflowContext) *models.WorkflowResult {
	result := &models.WorkflowResult{
		Status:      models.ExecutionStatusSuccess,
		NodeResults: make([]*models.NodeExecutionResult, 0),
		Stream:      wfCtx.Stream, // 从上下文继承流式输出设置
	}

	// 计算每个节点的入度
	inDegree := make(map[string]int)
	for nodeID := range nodeMap {
		inDegree[nodeID] = 0
	}

	// 遍历连接，计算入度
	for _, nodeConns := range connections {
		for _, targets := range nodeConns.Main {
			for _, target := range targets {
				inDegree[target.Node]++
			}
		}
	}

	// 找到所有入度为 0 的节点（起始节点）
	queue := make([]string, 0)
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	// 如果没有起始节点，说明存在循环依赖
	if len(queue) == 0 && len(nodeMap) > 0 {
		result.Status = models.ExecutionStatusFailed
		result.Message = "workflow has circular dependency"
		return result
	}

	// 记录已执行的节点
	executed := make(map[string]bool)
	// 记录节点的分支选择结果
	branchResults := make(map[string]*int)

	for len(queue) > 0 {
		// 取出队首节点
		nodeID := queue[0]
		queue = queue[1:]

		// 检查是否已执行
		if executed[nodeID] {
			continue
		}

		node, exists := nodeMap[nodeID]
		if !exists {
			continue
		}

		// 执行节点
		nodeResult, nodeOutput := e.executeNode(node, wfCtx)
		result.NodeResults = append(result.NodeResults, nodeResult)

		if nodeOutput != nil && nodeOutput.Stream && nodeOutput.StreamChan != nil {
			// 流式输出节点通常是最后一个节点
			// 直接传递 StreamChan 给 WorkflowResult，不阻塞等待
			result.Stream = true
			result.StreamChan = nodeOutput.StreamChan
			result.Event = wfCtx.Event
			result.Status = "streaming"
			result.Message = fmt.Sprintf("streaming output from node: %s", node.Name)

			// 更新节点状态为 streaming
			nodeResult.Status = "streaming"
			nodeResult.Message = "streaming in progress"

			// 立即返回，让 API 层处理流式响应
			return result
		}
		executed[nodeID] = true

		// 保存分支结果
		if nodeResult.BranchIndex != nil {
			branchResults[nodeID] = nodeResult.BranchIndex
		}

		// 检查执行状态
		if nodeResult.Status == "failed" {
			if !node.ContinueOnFail {
				result.Status = models.ExecutionStatusFailed
				result.ErrorNode = nodeID
				result.Message = fmt.Sprintf("node %s failed: %s", node.Name, nodeResult.Error)
			}
		}

		// 检查是否终止
		if nodeResult.Status == "terminated" {
			result.Message = fmt.Sprintf("workflow terminated at node %s", node.Name)
			return result
		}

		// 更新后继节点的入度
		if nodeConns, ok := connections[nodeID]; ok {
			for outputIndex, targets := range nodeConns.Main {
				// 检查是否应该走这个分支
				if !e.shouldFollowBranch(nodeID, outputIndex, branchResults) {
					continue
				}

				for _, target := range targets {
					inDegree[target.Node]--
					if inDegree[target.Node] == 0 {
						queue = append(queue, target.Node)
					}
				}
			}
		}
	}

	return result
}

// executeNode 执行单个节点
// 返回：节点执行结果、节点输出（用于流式输出检测）
func (e *WorkflowEngine) executeNode(node *models.WorkflowNode, wfCtx *models.WorkflowContext) (*models.NodeExecutionResult, *models.NodeOutput) {
	startTime := time.Now()
	nodeResult := &models.NodeExecutionResult{
		NodeID:    node.ID,
		NodeName:  node.Name,
		NodeType:  node.Type,
		StartedAt: startTime.Unix(),
	}

	var nodeOutput *models.NodeOutput

	// 跳过禁用的节点
	if node.Disabled {
		nodeResult.Status = "skipped"
		nodeResult.Message = "node is disabled"
		nodeResult.FinishedAt = time.Now().Unix()
		nodeResult.DurationMs = time.Since(startTime).Milliseconds()
		return nodeResult, nil
	}

	// 获取处理器
	processor, err := models.GetProcessorByType(node.Type, node.Config)
	if err != nil {
		nodeResult.Status = "failed"
		nodeResult.Error = fmt.Sprintf("failed to get processor: %v", err)
		nodeResult.FinishedAt = time.Now().Unix()
		nodeResult.DurationMs = time.Since(startTime).Milliseconds()
		return nodeResult, nil
	}

	// 分段执行时跳过不属于本段的节点（透传，路由走默认输出）：
	// 变换段跳过动作节点（callback/ai_summary 推迟到事件真正产生时执行），
	// 动作段跳过变换节点（已在评估段执行过，事件上已带着变换结果）
	if wfCtx != nil && !models.ProcessorRunsInStage(processor, wfCtx.Stage) {
		nodeResult.Status = "skipped"
		nodeResult.SkippedByStage = true
		if wfCtx.Stage == models.StageTransform {
			nodeResult.Message = "deferred to action stage, runs when event is produced"
		} else {
			nodeResult.Message = "already executed in transform stage during evaluation"
		}
		nodeResult.FinishedAt = time.Now().Unix()
		nodeResult.DurationMs = time.Since(startTime).Milliseconds()
		return nodeResult, nil
	}

	// 处理器执行前拍一份精简快照，用于前后对比。
	// relabel 等处理器会就地修改事件，必须在执行前把快照固化为字符串。
	beforeSnap := ""
	if wfCtx != nil && wfCtx.Event != nil {
		beforeSnap = wfCtx.Event.LoggableSnapshot()
	}

	// 执行处理器（带重试）
	var retries int
	dropped := false // 事件是否在本节点被 drop
	maxRetries := node.MaxRetries
	if !node.RetryOnFail {
		maxRetries = 0
	}

	for retries <= maxRetries {
		// 检查是否为分支处理器
		if branchProcessor, ok := processor.(models.BranchProcessor); ok {
			output, err := branchProcessor.ProcessWithBranch(e.ctx, wfCtx)
			if err != nil {
				if retries < maxRetries {
					retries++
					time.Sleep(time.Duration(node.RetryInterval) * time.Second)
					continue
				}
				nodeResult.Status = "failed"
				nodeResult.Error = err.Error()
			} else {
				nodeResult.Status = "success"
				if output != nil {
					nodeOutput = output
					if output.WfCtx != nil {
						wfCtx = output.WfCtx
					}
					nodeResult.Message = output.Message
					nodeResult.BranchIndex = output.BranchIndex
					if output.Terminate {
						nodeResult.Status = "terminated"
					}
				}
			}
			break
		}

		// 普通处理器
		newWfCtx, msg, err := processor.Process(e.ctx, wfCtx)
		if err != nil {
			if retries < maxRetries {
				retries++
				time.Sleep(time.Duration(node.RetryInterval) * time.Second)
				continue
			}
			nodeResult.Status = "failed"
			nodeResult.Error = err.Error()
		} else {
			nodeResult.Status = "success"
			nodeResult.Message = msg
			if newWfCtx != nil {
				wfCtx = newWfCtx

				// 检测流式输出标记
				if newWfCtx.Stream && newWfCtx.StreamChan != nil {
					nodeOutput = &models.NodeOutput{
						WfCtx:      newWfCtx,
						Message:    msg,
						Stream:     true,
						StreamChan: newWfCtx.StreamChan,
					}
				}
			}

			// 如果事件被 drop（返回 nil 或 Event 为 nil），标记为终止
			if newWfCtx == nil || newWfCtx.Event == nil {
				nodeResult.Status = "terminated"
				nodeResult.Message = msg
				dropped = true
			}
		}
		break
	}

	nodeResult.FinishedAt = time.Now().Unix()
	nodeResult.DurationMs = time.Since(startTime).Milliseconds()

	// 处理器执行后拍快照并与执行前对比，得到本节点对事件的字段级改动。
	// dropped 时 afterSnap 为空，diff 会记为「事件已丢弃」。
	afterSnap := ""
	if !dropped && wfCtx != nil && wfCtx.Event != nil {
		afterSnap = wfCtx.Event.LoggableSnapshot()
	}
	changeStr := models.FormatEventChanges(models.DiffEventSnapshot(beforeSnap, afterSnap, dropped))

	// 复用 Message 字段承载改动信息（前端执行记录已渲染 message，无需新增字段/迁移/前端改动），
	// 保留处理器自身返回的 msg（如 event_drop 的丢弃原因）。skipped/failed 节点不追加。
	if nodeResult.Status != "skipped" && nodeResult.Status != "failed" {
		if nodeResult.Message != "" {
			nodeResult.Message = nodeResult.Message + " | " + changeStr
		} else {
			nodeResult.Message = changeStr
		}
	}

	var eventHash string
	if wfCtx != nil && wfCtx.Event != nil {
		eventHash = wfCtx.Event.Hash
	}

	var pipelineID string
	if wfCtx != nil && wfCtx.Metadata != nil {
		pipelineID = wfCtx.Metadata["pipeline_id"]
	}

	// Message 已包含处理器自身返回的 msg 与改动信息（msg | changes）；failed 节点 Message 为空、
	// 失败原因在 Error，故一并打出，避免日志里看不到失败/丢弃原因。
	// 变换段每个评估周期都会执行，节点级日志降为 debug，避免按评估频率刷日志
	nodeLogf := logger.Infof
	if wfCtx != nil && wfCtx.Stage == models.StageTransform {
		nodeLogf = logger.Debugf
	}
	nodeLogf("workflow: node name=%s type=%s status=%s duration=%dms event_hash=%s pipeline_id=%s msg=%q error=%q",
		node.Name, node.Type, nodeResult.Status, nodeResult.DurationMs, eventHash, pipelineID, nodeResult.Message, nodeResult.Error)
	logger.Debugf("workflow: node name=%s before=%s after=%s", node.Name, beforeSnap, afterSnap)

	return nodeResult, nodeOutput
}

// shouldFollowBranch 判断是否应该走某个分支
func (e *WorkflowEngine) shouldFollowBranch(nodeID string, outputIndex int, branchResults map[string]*int) bool {
	branchIndex, hasBranch := branchResults[nodeID]
	if !hasBranch {
		// 没有分支结果，说明不是分支节点，只走第一个输出
		return outputIndex == 0
	}

	if branchIndex == nil {
		// branchIndex 为 nil，走默认分支（通常是最后一个）
		return true
	}

	// 只走选中的分支
	return outputIndex == *branchIndex
}

func (e *WorkflowEngine) saveExecutionRecord(pipeline *models.EventPipeline, wfCtx *models.WorkflowContext, result *models.WorkflowResult, triggerCtx *models.WorkflowTriggerContext, startTime int64, duration int64) {
	executionID := triggerCtx.RequestID
	if executionID == "" {
		executionID = uuid.New().String()
	}

	execution := &models.EventPipelineExecution{
		ID:           executionID,
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Mode:         triggerCtx.Mode,
		Status:       result.Status,
		ErrorMessage: result.Message,
		ErrorNode:    result.ErrorNode,
		CreatedAt:    startTime,
		FinishedAt:   time.Now().Unix(),
		DurationMs:   duration,
		TriggerBy:    triggerCtx.TriggerBy,
	}

	if wfCtx.Event != nil {
		execution.EventID = wfCtx.Event.Id
	}

	if err := execution.SetNodeResults(result.NodeResults); err != nil {
		logger.Errorf("workflow: failed to set node results: pipeline_id=%d, error=%v", pipeline.ID, err)
	}

	if err := execution.SetInputsSnapshot(wfCtx.Inputs); err != nil {
		logger.Errorf("workflow: failed to set inputs snapshot: pipeline_id=%d, error=%v", pipeline.ID, err)
	}

	if err := models.CreateEventPipelineExecution(e.ctx, execution); err != nil {
		logger.Errorf("workflow: failed to save execution record: pipeline_id=%d, error=%v", pipeline.ID, err)
	}
}
