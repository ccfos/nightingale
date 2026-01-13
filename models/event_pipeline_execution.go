package models

import (
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// 执行状态常量
const (
	ExecutionStatusRunning = "running"
	ExecutionStatusSuccess = "success"
	ExecutionStatusFailed  = "failed"
)

// EventPipelineExecution 工作流执行记录
type EventPipelineExecution struct {
	ID           string `json:"id" gorm:"primaryKey;type:varchar(36)"`
	PipelineID   int64  `json:"pipeline_id" gorm:"index"`
	PipelineName string `json:"pipeline_name" gorm:"type:varchar(128)"`
	EventID      int64  `json:"event_id" gorm:"index"`

	// 触发模式：event（告警触发）、api（API触发）、cron（定时触发）
	Mode string `json:"mode" gorm:"type:varchar(16);index"`

	// 状态：running、success、failed
	Status string `json:"status" gorm:"type:varchar(16);index"`

	// 各节点执行结果（JSON）
	NodeResults string `json:"node_results" gorm:"type:mediumtext"`

	// 错误信息
	ErrorMessage string `json:"error_message" gorm:"type:varchar(1024)"`
	ErrorNode    string `json:"error_node" gorm:"type:varchar(36)"`

	// 时间
	CreatedAt  int64 `json:"created_at" gorm:"index"`
	FinishedAt int64 `json:"finished_at"`
	DurationMs int64 `json:"duration_ms"`

	// 触发者信息
	TriggerBy string `json:"trigger_by" gorm:"type:varchar(64)"`

	// 环境变量快照（脱敏后存储）
	EnvSnapshot string `json:"env_snapshot,omitempty" gorm:"type:text"`
}

func (e *EventPipelineExecution) TableName() string {
	return "event_pipeline_execution"
}

// SetNodeResults 设置节点执行结果（序列化为 JSON）
func (e *EventPipelineExecution) SetNodeResults(results []*NodeExecutionResult) error {
	data, err := json.Marshal(results)
	if err != nil {
		return err
	}
	e.NodeResults = string(data)
	return nil
}

// GetNodeResults 获取节点执行结果（反序列化）
func (e *EventPipelineExecution) GetNodeResults() ([]*NodeExecutionResult, error) {
	if e.NodeResults == "" {
		return nil, nil
	}
	var results []*NodeExecutionResult
	err := json.Unmarshal([]byte(e.NodeResults), &results)
	return results, err
}

// SetEnvSnapshot 设置环境变量快照（脱敏后存储）
func (e *EventPipelineExecution) SetEnvSnapshot(env map[string]string) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	e.EnvSnapshot = string(data)
	return nil
}

// GetEnvSnapshot 获取环境变量快照
func (e *EventPipelineExecution) GetEnvSnapshot() (map[string]string, error) {
	if e.EnvSnapshot == "" {
		return nil, nil
	}
	var env map[string]string
	err := json.Unmarshal([]byte(e.EnvSnapshot), &env)
	return env, err
}

// CreateEventPipelineExecution 创建执行记录
func CreateEventPipelineExecution(c *ctx.Context, execution *EventPipelineExecution) error {
	return DB(c).Create(execution).Error
}

// UpdateEventPipelineExecution 更新执行记录
func UpdateEventPipelineExecution(c *ctx.Context, execution *EventPipelineExecution) error {
	return DB(c).Save(execution).Error
}

// GetEventPipelineExecution 获取单条执行记录
func GetEventPipelineExecution(c *ctx.Context, id string) (*EventPipelineExecution, error) {
	var execution EventPipelineExecution
	err := DB(c).Where("id = ?", id).First(&execution).Error
	if err != nil {
		return nil, err
	}
	return &execution, nil
}

// ListEventPipelineExecutions 获取 Pipeline 的执行记录列表
func ListEventPipelineExecutions(c *ctx.Context, pipelineID int64, mode, status string, limit, offset int) ([]*EventPipelineExecution, int64, error) {
	var executions []*EventPipelineExecution
	var total int64

	session := DB(c).Model(&EventPipelineExecution{}).Where("pipeline_id = ?", pipelineID)

	if mode != "" {
		session = session.Where("mode = ?", mode)
	}
	if status != "" {
		session = session.Where("status = ?", status)
	}

	err := session.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = session.Order("created_at desc").Limit(limit).Offset(offset).Find(&executions).Error
	if err != nil {
		return nil, 0, err
	}

	return executions, total, nil
}

// ListEventPipelineExecutionsByEventID 根据事件ID获取执行记录
func ListEventPipelineExecutionsByEventID(c *ctx.Context, eventID int64) ([]*EventPipelineExecution, error) {
	var executions []*EventPipelineExecution
	err := DB(c).Where("event_id = ?", eventID).Order("created_at desc").Find(&executions).Error
	return executions, err
}

// ListAllEventPipelineExecutions 获取所有 Pipeline 的执行记录列表
func ListAllEventPipelineExecutions(c *ctx.Context, pipelineName, mode, status string, limit, offset int) ([]*EventPipelineExecution, int64, error) {
	var executions []*EventPipelineExecution
	var total int64

	session := DB(c).Model(&EventPipelineExecution{})

	if pipelineName != "" {
		session = session.Where("pipeline_name LIKE ?", "%"+pipelineName+"%")
	}
	if mode != "" {
		session = session.Where("mode = ?", mode)
	}
	if status != "" {
		session = session.Where("status = ?", status)
	}

	err := session.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = session.Order("created_at desc").Limit(limit).Offset(offset).Find(&executions).Error
	if err != nil {
		return nil, 0, err
	}

	return executions, total, nil
}

// DeleteEventPipelineExecutions 批量删除执行记录（按时间）
func DeleteEventPipelineExecutions(c *ctx.Context, beforeTime int64) (int64, error) {
	result := DB(c).Where("created_at < ?", beforeTime).Delete(&EventPipelineExecution{})
	return result.RowsAffected, result.Error
}

// DeleteEventPipelineExecutionsInBatches 分批删除执行记录（按时间）
// 每次删除 limit 条记录，返回本次删除的数量
func DeleteEventPipelineExecutionsInBatches(c *ctx.Context, beforeTime int64, limit int) (int64, error) {
	result := DB(c).Where("created_at < ?", beforeTime).Limit(limit).Delete(&EventPipelineExecution{})
	return result.RowsAffected, result.Error
}

// DeleteEventPipelineExecutionsByPipelineID 删除指定 Pipeline 的所有执行记录
func DeleteEventPipelineExecutionsByPipelineID(c *ctx.Context, pipelineID int64) error {
	return DB(c).Where("pipeline_id = ?", pipelineID).Delete(&EventPipelineExecution{}).Error
}

// EventPipelineExecutionStatistics 执行统计
type EventPipelineExecutionStatistics struct {
	Total     int64 `json:"total"`
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
	Running   int64 `json:"running"`
	AvgDurMs  int64 `json:"avg_duration_ms"`
	LastRunAt int64 `json:"last_run_at"`
}

// GetEventPipelineExecutionStatistics 获取执行统计信息
func GetEventPipelineExecutionStatistics(c *ctx.Context, pipelineID int64) (*EventPipelineExecutionStatistics, error) {
	var stats EventPipelineExecutionStatistics

	// 总数
	err := DB(c).Model(&EventPipelineExecution{}).Where("pipeline_id = ?", pipelineID).Count(&stats.Total).Error
	if err != nil {
		return nil, err
	}

	// 成功数
	err = DB(c).Model(&EventPipelineExecution{}).Where("pipeline_id = ? AND status = ?", pipelineID, ExecutionStatusSuccess).Count(&stats.Success).Error
	if err != nil {
		return nil, err
	}

	// 失败数
	err = DB(c).Model(&EventPipelineExecution{}).Where("pipeline_id = ? AND status = ?", pipelineID, ExecutionStatusFailed).Count(&stats.Failed).Error
	if err != nil {
		return nil, err
	}

	// 运行中
	err = DB(c).Model(&EventPipelineExecution{}).Where("pipeline_id = ? AND status = ?", pipelineID, ExecutionStatusRunning).Count(&stats.Running).Error
	if err != nil {
		return nil, err
	}

	// 平均耗时
	var avgDur struct {
		AvgDur float64 `gorm:"column:avg_dur"`
	}
	err = DB(c).Model(&EventPipelineExecution{}).
		Select("AVG(duration_ms) as avg_dur").
		Where("pipeline_id = ? AND status = ?", pipelineID, ExecutionStatusSuccess).
		Scan(&avgDur).Error
	if err != nil {
		return nil, err
	}
	stats.AvgDurMs = int64(avgDur.AvgDur)

	// 最后执行时间
	var lastExec EventPipelineExecution
	err = DB(c).Where("pipeline_id = ?", pipelineID).Order("created_at desc").First(&lastExec).Error
	if err == nil {
		stats.LastRunAt = lastExec.CreatedAt
	}

	return &stats, nil
}

// EventPipelineExecutionDetail 执行详情（包含解析后的节点结果）
type EventPipelineExecutionDetail struct {
	EventPipelineExecution
	NodeResultsParsed []*NodeExecutionResult `json:"node_results_parsed"`
	EnvSnapshotParsed map[string]string      `json:"env_snapshot_parsed"`
}

// GetEventPipelineExecutionDetail 获取执行详情
func GetEventPipelineExecutionDetail(c *ctx.Context, id string) (*EventPipelineExecutionDetail, error) {
	execution, err := GetEventPipelineExecution(c, id)
	if err != nil {
		return nil, err
	}

	detail := &EventPipelineExecutionDetail{
		EventPipelineExecution: *execution,
	}

	// 解析节点结果
	nodeResults, err := execution.GetNodeResults()
	if err != nil {
		return nil, fmt.Errorf("parse node results error: %w", err)
	}
	detail.NodeResultsParsed = nodeResults

	// 解析环境变量快照
	envSnapshot, err := execution.GetEnvSnapshot()
	if err != nil {
		return nil, fmt.Errorf("parse env snapshot error: %w", err)
	}
	detail.EnvSnapshotParsed = envSnapshot

	return detail, nil
}
