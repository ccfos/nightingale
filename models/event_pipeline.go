package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

// EventPipeline 事件Pipeline模型
type EventPipeline struct {
	ID               int64             `json:"id" gorm:"primaryKey"`
	Name             string            `json:"name" gorm:"type:varchar(128)"`
	Typ              string            `json:"typ" gorm:"type:varchar(128)"`          // builtin, user-defined    // event_pipeline, event_summary, metric_explorer
	UseCase          string            `json:"use_case" gorm:"type:varchar(128)"`     // metric_explorer, event_summary, event_pipeline
	TriggerMode      string            `json:"trigger_mode" gorm:"type:varchar(128)"` // event, api, cron
	Disabled         bool              `json:"disabled" gorm:"type:boolean"`
	TeamIds          []int64           `json:"team_ids" gorm:"type:text;serializer:json"`
	TeamNames        []string          `json:"team_names" gorm:"-"`
	Description      string            `json:"description" gorm:"type:varchar(255)"`
	FilterEnable     bool              `json:"filter_enable" gorm:"type:boolean"`
	LabelFilters     []TagFilter       `json:"label_filters" gorm:"type:text;serializer:json"`
	AttrFilters      []TagFilter       `json:"attribute_filters" gorm:"type:text;serializer:json"`
	ProcessorConfigs []ProcessorConfig `json:"processors" gorm:"type:text;serializer:json"`

	// 工作流节点列表
	Nodes []WorkflowNode `json:"nodes,omitempty" gorm:"type:text;serializer:json"`
	// 节点连接关系
	Connections Connections `json:"connections,omitempty" gorm:"type:text;serializer:json"`
	// 环境变量（工作流级别的配置变量）
	EnvVariables []EnvVariable `json:"env_variables,omitempty" gorm:"type:text;serializer:json"`

	CreateAt int64  `json:"create_at" gorm:"type:bigint"`
	CreateBy string `json:"create_by" gorm:"type:varchar(64)"`
	UpdateAt int64  `json:"update_at" gorm:"type:bigint"`
	UpdateBy string `json:"update_by" gorm:"type:varchar(64)"`
}

type ProcessorConfig struct {
	Typ    string      `json:"typ"`
	Config interface{} `json:"config"`
}

func (e *EventPipeline) TableName() string {
	return "event_pipeline"
}

func (e *EventPipeline) Verify() error {
	if e.Name == "" {
		return errors.New("name cannot be empty")
	}

	if len(e.TeamIds) == 0 {
		return errors.New("team_ids cannot be empty")
	}

	if len(e.LabelFilters) == 0 {
		e.LabelFilters = make([]TagFilter, 0)
	}
	if len(e.AttrFilters) == 0 {
		e.AttrFilters = make([]TagFilter, 0)
	}
	if len(e.ProcessorConfigs) == 0 {
		e.ProcessorConfigs = make([]ProcessorConfig, 0)
	}

	// 初始化空数组，避免 null
	if e.Nodes == nil {
		e.Nodes = make([]WorkflowNode, 0)
	}
	if e.Connections == nil {
		e.Connections = make(Connections)
	}
	if e.EnvVariables == nil {
		e.EnvVariables = make([]EnvVariable, 0)
	}

	return nil
}

// CreateEventPipeline 创建事件Pipeline
func CreateEventPipeline(ctx *ctx.Context, pipeline *EventPipeline) error {
	return DB(ctx).Create(pipeline).Error
}

// GetEventPipeline 获取单个事件Pipeline
func GetEventPipeline(ctx *ctx.Context, id int64) (*EventPipeline, error) {
	var pipeline EventPipeline
	err := DB(ctx).Where("id = ?", id).First(&pipeline).Error
	if err != nil {
		return nil, err
	}
	pipeline.Verify()
	return &pipeline, nil
}

func GetEventPipelinesByIds(ctx *ctx.Context, ids []int64) ([]*EventPipeline, error) {
	var pipelines []*EventPipeline
	err := DB(ctx).Where("id in ?", ids).Find(&pipelines).Error
	return pipelines, err
}

// UpdateEventPipeline 更新事件Pipeline
func UpdateEventPipeline(ctx *ctx.Context, pipeline *EventPipeline) error {
	return DB(ctx).Save(pipeline).Error
}

// DeleteEventPipeline 删除事件Pipeline
func DeleteEventPipeline(ctx *ctx.Context, id int64) error {
	return DB(ctx).Delete(&EventPipeline{}, id).Error
}

// ListEventPipelines 获取事件Pipeline列表
func ListEventPipelines(ctx *ctx.Context) ([]*EventPipeline, error) {
	if !ctx.IsCenter {
		pipelines, err := poster.GetByUrls[[]*EventPipeline](ctx, "/v1/n9e/event-pipelines")
		return pipelines, err
	}

	var pipelines []*EventPipeline
	err := DB(ctx).Order("name asc").Find(&pipelines).Error
	if err != nil {
		return nil, err
	}

	for _, p := range pipelines {
		p.Verify()
	}

	return pipelines, nil
}

// DeleteEventPipelines 批量删除事件Pipeline
func DeleteEventPipelines(ctx *ctx.Context, ids []int64) error {
	return DB(ctx).Where("id in ?", ids).Delete(&EventPipeline{}).Error
}

// Update 更新事件Pipeline
func (e *EventPipeline) Update(ctx *ctx.Context, ref *EventPipeline) error {
	ref.ID = e.ID
	ref.CreateAt = e.CreateAt
	ref.CreateBy = e.CreateBy
	ref.UpdateAt = time.Now().Unix()

	err := ref.Verify()
	if err != nil {
		return err
	}

	return DB(ctx).Model(e).Select("*").Updates(*ref).Error
}

// FillTeamNames 填充团队名称
func (e *EventPipeline) FillTeamNames(ctx *ctx.Context) error {
	e.TeamNames = make([]string, 0, len(e.TeamIds))
	if len(e.TeamIds) == 0 {
		return nil
	}

	teamMap, err := UserGroupIdAndNameMap(ctx, e.TeamIds)
	if err != nil {
		return err
	}

	// 按原始TeamIds顺序填充TeamNames
	for _, tid := range e.TeamIds {
		if name, exists := teamMap[tid]; exists {
			e.TeamNames = append(e.TeamNames, name)
		}
	}

	return nil
}

func EventPipelineStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=event_pipeline")
		return s, err
	}

	session := DB(ctx).Model(&EventPipeline{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	if len(stats) == 0 {
		return nil, fmt.Errorf("no event pipeline found")
	}

	return stats[0], nil
}

// 无论是新格式还是旧格式，都返回统一的 []WorkflowNode
func (e *EventPipeline) GetWorkflowNodes() []WorkflowNode {
	// 优先使用新格式
	if len(e.Nodes) > 0 {
		return e.Nodes
	}

	// 兼容旧格式：将 ProcessorConfigs 转换为 WorkflowNode
	nodes := make([]WorkflowNode, len(e.ProcessorConfigs))
	for i, pc := range e.ProcessorConfigs {
		nodeID := fmt.Sprintf("node_%d", i)
		nodeName := pc.Typ

		nodes[i] = WorkflowNode{
			ID:     nodeID,
			Name:   nodeName,
			Type:   pc.Typ,
			Config: pc.Config,
		}
	}
	return nodes
}

func (e *EventPipeline) GetWorkflowConnections() Connections {
	// 优先使用显式定义的连接
	if len(e.Connections) > 0 {
		return e.Connections
	}

	// 自动生成线性连接：node_0 → node_1 → node_2 → ...
	nodes := e.GetWorkflowNodes()
	conns := make(Connections)

	for i := 0; i < len(nodes)-1; i++ {
		conns[nodes[i].ID] = NodeConnections{
			Main: [][]ConnectionTarget{
				{{Node: nodes[i+1].ID, Type: "main", Index: 0}},
			},
		}
	}
	return conns
}

func (e *EventPipeline) FillWorkflowFields() {
	if len(e.Nodes) == 0 && len(e.ProcessorConfigs) > 0 {
		e.Nodes = e.GetWorkflowNodes()
		e.Connections = e.GetWorkflowConnections()
	}
}

func (e *EventPipeline) GetEnvMap() map[string]string {
	envMap := make(map[string]string)
	for _, v := range e.EnvVariables {
		envMap[v.Key] = v.Value
	}
	return envMap
}

func (e *EventPipeline) GetSecretKeys() map[string]bool {
	secretKeys := make(map[string]bool)
	for _, v := range e.EnvVariables {
		if v.Secret {
			secretKeys[v.Key] = true
		}
	}
	return secretKeys
}

func (e *EventPipeline) ValidateEnvVariables(overrides map[string]string) error {
	// 合并默认值和覆盖值
	merged := e.GetEnvMap()
	for k, v := range overrides {
		merged[k] = v
	}

	// 校验必填项
	for _, v := range e.EnvVariables {
		if v.Required && merged[v.Key] == "" {
			return fmt.Errorf("required env variable %s is missing", v.Key)
		}
	}
	return nil
}
