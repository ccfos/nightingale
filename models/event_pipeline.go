package models

import (
	"errors"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

// EventPipeline 事件Pipeline模型
type EventPipeline struct {
	ID           int64       `json:"id" gorm:"primaryKey"`
	Name         string      `json:"name" gorm:"type:varchar(128)"`
	TeamIds      []int64     `json:"team_ids" gorm:"type:text;serializer:json"`
	TeamNames    []string    `json:"team_names" gorm:"-"`
	Description  string      `json:"description" gorm:"type:varchar(255)"`
	FilterEnable bool        `json:"filter_enable" gorm:"type:tinyint(1)"`
	LabelFilters []TagFilter `json:"label_filters" gorm:"type:text;serializer:json"`
	AttrFilters  []TagFilter `json:"attribute_filters" gorm:"type:text;serializer:json"`
	Processors   []Processor `json:"processors" gorm:"type:text;serializer:json"`
	CreatedAt    int64       `json:"created_at" gorm:"type:bigint"`
	CreatedBy    string      `json:"created_by" gorm:"type:varchar(64)"`
	UpdateAt     int64       `json:"update_at" gorm:"type:bigint"`
	UpdateBy     string      `json:"update_by" gorm:"type:varchar(64)"`
}

// Processor 处理器配置
type Processor struct {
	Typ    string      `json:"typ"`
	Config interface{} `json:"config"`
}

// LabelEnrichProcessor 标签丰富处理器配置
type LabelEnrichProcessor struct {
	LabelSourceType string             `json:"label_source_type"`
	LabelMappingID  string             `json:"label_mapping_id"`
	SourceKeys      []LabelMappingPair `json:"source_keys"`
	AppendKeys      []LabelMappingPair `json:"append_keys"`
}

// LabelMappingPair 标签映射对
type LabelMappingPair struct {
	SourceKey string `json:"source_key"`
	TargetKey string `json:"target_key"`
	RenameKey bool   `json:"rename_key,omitempty"`
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

	return nil
}

func (e *EventPipeline) DB2FE() {
	if e.TeamIds == nil {
		e.TeamIds = make([]int64, 0)
	}
	if e.LabelFilters == nil {
		e.LabelFilters = make([]TagFilter, 0)
	}
	if e.AttrFilters == nil {
		e.AttrFilters = make([]TagFilter, 0)
	}
	if e.Processors == nil {
		e.Processors = make([]Processor, 0)
	}
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
	pipeline.DB2FE()
	return &pipeline, nil
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
	err := DB(ctx).Find(&pipelines).Error
	if err != nil {
		return nil, err
	}

	for _, p := range pipelines {
		p.DB2FE()
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
	ref.CreatedAt = e.CreatedAt
	ref.CreatedBy = e.CreatedBy
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
	for _, tid := range e.TeamIds {
		team, err := UserGroupGet(ctx, "id = ?", tid)
		if err != nil {
			continue
		}

		if team == nil {
			continue
		}

		e.TeamNames = append(e.TeamNames, team.Name)
	}
	return nil
}
