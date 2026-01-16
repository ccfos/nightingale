package engine

import (
	"fmt"
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

	duration := time.Since(startTime).Milliseconds()

	if triggerCtx != nil && triggerCtx.Mode != "" {
		e.saveExecutionRecord(pipeline, wfCtx, result, triggerCtx, startTime.Unix(), duration)
	}

	return wfCtx.Event, result, nil
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
	if triggerCtx != nil {
		metadata["request_id"] = triggerCtx.RequestID
		metadata["trigger_mode"] = triggerCtx.Mode
		metadata["trigger_by"] = triggerCtx.TriggerBy
		stream = triggerCtx.Stream
	}

	return &models.WorkflowContext{
		Event:    event,
		Inputs:   inputs,
		Vars:     make(map[string]interface{}), // 初始化空的 Vars，供节点间传递数据
		Metadata: metadata,
		Stream:   stream,
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
				return result
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

	// 执行处理器（带重试）
	var retries int
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
			}
		}
		break
	}

	nodeResult.FinishedAt = time.Now().Unix()
	nodeResult.DurationMs = time.Since(startTime).Milliseconds()

	logger.Infof("workflow: executed node %s (type=%s) status=%s msg=%s duration=%dms",
		node.Name, node.Type, nodeResult.Status, nodeResult.Message, nodeResult.DurationMs)

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
