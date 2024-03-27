package models

import (
	"errors"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"strings"
	"time"
)

// BuiltinMetric represents a metric along with its metadata.
type BuiltinMetric struct {
	ID         int64  `json:"id" gorm:"primaryKey"` // Unique identifier
	Collector  string `json:"collector"`            // Typ of collector (e.g., 'categraf', 'telegraf')
	Typ        string `json:"type"`                 // Typ of metric (e.g., 'host', 'mysql', 'redis')
	Name       string `json:"name"`                 // Name of the metric
	Unit       string `json:"unit"`                 // Unit of the metric
	DescCN     string `json:"desc_cn"`              // Description in Chinese
	DescEN     string `json:"desc_en"`              // Description in English
	Expression string `json:"expression"`           // Expression for calculation
	CreatedAt  int64  `json:"created_at"`           // Creation timestamp (unix time)
	CreatedBy  int64  `json:"created_by"`           // Creator
	UpdatedAt  int64  `json:"updated_at"`           // Update timestamp (unix time)
}

// TableName returns the table name of the BuiltinMetric model.
func (bm *BuiltinMetric) TableName() string {
	return "builtin_metrics"
}

// DB2FE Convert frontend (FE) fields to database format if necessary.
func (bm *BuiltinMetric) DB2FE() error {
	return nil
}

// Verify the BuiltinMetric fields.
func (bm *BuiltinMetric) Verify() error {
	bm.Collector = strings.TrimSpace(bm.Collector)
	if bm.Collector == "" {
		return errors.New("collector is blank")
	}

	bm.Typ = strings.TrimSpace(bm.Typ)
	if bm.Typ == "" {
		return errors.New("type is blank")
	}

	bm.Name = strings.TrimSpace(bm.Name)
	if bm.Name == "" {
		return errors.New("name is blank")
	}

	return nil
}

// BuiltinMetricExists Check if a BuiltinMetric already exists.
func BuiltinMetricExists(ctx *ctx.Context, bm *BuiltinMetric) (bool, error) {
	var count int64
	err := DB(ctx).Model(bm).Where("collector = ? and type = ? and name = ?", bm.Collector, bm.Typ, bm.Name).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Add a BuiltinMetric, considering safe insert conditions.
func (bm *BuiltinMetric) Add(ctx *ctx.Context) error {
	if err := bm.Verify(); err != nil {
		return err
	}
	// check if the builtin metric already exists
	exists, err := BuiltinMetricExists(ctx, bm)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("builtin metric already exists")
	}
	now := time.Now().Unix()
	bm.CreatedAt = now
	bm.UpdatedAt = now
	return Insert(ctx, bm)
}

// Update a BuiltinMetric, considering safe update conditions.
func (bm *BuiltinMetric) Update(ctx *ctx.Context, Collector, Type, Name, Unit, DescCN, DescEN, Expression string, createdBy int64) error {
	if err := bm.Verify(); err != nil {
		return err
	}
	now := time.Now().Unix()
	bm.UpdatedAt = now
	bm.CreatedBy = createdBy

	bm.Collector = Collector
	bm.Typ = Type
	bm.Name = Name
	bm.Unit = Unit
	bm.DescCN = DescCN
	bm.DescEN = DescEN
	bm.Expression = Expression
	return DB(ctx).Model(bm).Select("collector", "type", "name", "unit", "desc_cn", "desc_en", "expression", "updated_at", "created_by").Updates(bm).Error
}

// BuiltinMetricDel Delete a BuiltinMetric, considering safe delete conditions.
func BuiltinMetricDel(ctx *ctx.Context, ids []int64, createdBy ...interface{}) error {
	if len(ids) == 0 {
		return nil
	}

	if len(createdBy) > 0 {
		return DB(ctx).Where("id in ? AND created_by = ?", ids, createdBy[0]).Delete(new(BuiltinMetric)).Error
	}

	return DB(ctx).Where("id in ?", ids).Delete(new(BuiltinMetric)).Error
}

// BuiltinMetricGets Gets multiple BuiltinMetrics, optionally filtering by creator.
func BuiltinMetricGets(ctx *ctx.Context, collector, name, typ, descCn, descEn string, limit, offset int) ([]BuiltinMetric, error) {
	var lst []BuiltinMetric
	err := DB(ctx).Where("collector = ? or name like ? or typ = ? or desc_cn like ? or desc_en like ? ",
		collector, "%"+name+"%", typ, "%"+descCn+"%", "%"+descEn+"%").Limit(limit).Offset(offset).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	return lst, nil
}

// BuiltinMetricGet Get a single BuiltinMetric based on query parameters.
func BuiltinMetricGet(ctx *ctx.Context, where string, args ...interface{}) (*BuiltinMetric, error) {
	var lst []*BuiltinMetric
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}
