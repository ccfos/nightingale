package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm"
)

// BuiltinMetric represents a metric along with its metadata.
type BuiltinMetric struct {
	ID           int64         `json:"id" gorm:"primaryKey;type:bigint;autoIncrement;comment:'unique identifier'"`
	UUID         int64         `json:"uuid" gorm:"type:bigint;not null;default:0;comment:'uuid'"`
	Collector    string        `json:"collector" gorm:"type:varchar(191);not null;index:idx_collector,sort:asc;comment:'type of collector'"`
	Typ          string        `json:"typ" gorm:"type:varchar(191);not null;index:idx_typ,sort:asc;comment:'type of metric'"`
	Name         string        `json:"name" gorm:"type:varchar(191);not null;index:idx_builtinmetric_name,sort:asc;comment:'name of metric'"`
	Unit         string        `json:"unit" gorm:"type:varchar(191);not null;comment:'unit of metric'"`
	Note         string        `json:"note" gorm:"type:varchar(4096);not null;comment:'description of metric'"`
	Lang         string        `json:"lang" gorm:"type:varchar(191);not null;default:'zh';index:idx_lang,sort:asc;comment:'language'"`
	Translation  []Translation `json:"translation" gorm:"type:text;serializer:json;comment:'translation of metric'"`
	Expression   string        `json:"expression" gorm:"type:varchar(4096);not null;comment:'expression of metric'"`
	ExpressionType string            `json:"expression_type" gorm:"type:varchar(32);not null;default:'promql';comment:'expression type: metric_name or promql'"`
	MetricType     string            `json:"metric_type" gorm:"type:varchar(191);not null;default:'';comment:'metric type like counter/gauge'"`
	ExtraFields    map[string]string `json:"extra_fields" gorm:"type:text;serializer:json;comment:'custom extra fields'"`
	CreatedAt    int64         `json:"created_at" gorm:"type:bigint;not null;default:0;comment:'create time'"`
	CreatedBy    string        `json:"created_by" gorm:"type:varchar(191);not null;default:'';comment:'creator'"`
	UpdatedAt    int64         `json:"updated_at" gorm:"type:bigint;not null;default:0;comment:'update time'"`
	UpdatedBy    string        `json:"updated_by" gorm:"type:varchar(191);not null;default:'';comment:'updater'"`
}

type Translation struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
	Note string `json:"note"`
}

func (bm *BuiltinMetric) TableName() string {
	return "builtin_metrics"
}

func (bm *BuiltinMetric) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

func (bm *BuiltinMetric) Verify() error {
	if len(bm.Translation) == 0 {
		return errors.New("translation is required")
	}

	bm.Collector = strings.TrimSpace(bm.Collector)
	if bm.Collector == "" {
		return errors.New("collector is blank")
	}

	bm.Typ = strings.TrimSpace(bm.Typ)
	if bm.Typ == "" {
		return errors.New("type is blank")
	}

	return nil
}

func BuiltinMetricExists(ctx *ctx.Context, bm *BuiltinMetric) (bool, error) {
	var count int64
	err := DB(ctx).Model(bm).Where("expression = ? and collector = ? and typ = ?", bm.Expression, bm.Collector, bm.Typ).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (bm *BuiltinMetric) Add(ctx *ctx.Context, username string) error {
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
	bm.UpdatedBy = username
	bm.CreatedBy = username
	return Insert(ctx, bm)
}

func (bm *BuiltinMetric) Update(ctx *ctx.Context, req BuiltinMetric) error {
	if err := req.Verify(); err != nil {
		return err
	}

	req.UpdatedAt = time.Now().Unix()
	req.CreatedAt = bm.CreatedAt
	req.CreatedBy = bm.CreatedBy
	req.UUID = bm.UUID

	return DB(ctx).Model(bm).Select("*").Updates(req).Error
}

func BuiltinMetricDels(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	return DB(ctx).Where("id in ?", ids).Delete(new(BuiltinMetric)).Error
}

func BuiltinMetricGets(ctx *ctx.Context, lang, collector, typ, query, unit string) ([]*BuiltinMetric, error) {
	session := DB(ctx)
	session = builtinMetricQueryBuild(lang, collector, session, typ, query, unit)
	var lst []*BuiltinMetric
	if err := session.Order("collector asc, typ asc, name asc").Find(&lst).Error; err != nil {
		return nil, err
	}
	return lst, nil
}

func builtinMetricQueryBuild(lang, collector string, session *gorm.DB, typ string, query, unit string) *gorm.DB {
	session = session.Where("updated_by != ?", SYSTEM)

	if lang != "" {
		session = session.Where("lang = ?", lang)
	}

	if collector != "" {
		session = session.Where("collector = ?", collector)
	}

	if typ != "" {
		session = session.Where("typ = ?", typ)
	}

	if unit != "" {
		us := strings.Split(unit, ",")
		session = session.Where("unit in (?)", us)
	}

	if query != "" {
		qs := strings.Split(query, " ")

		for _, q := range qs {
			if strings.HasPrefix(q, "-") {
				q = strings.TrimPrefix(q, "-")
				queryPattern := "%" + q + "%"
				session = session.Where("name NOT LIKE ? AND note NOT LIKE ? AND expression NOT LIKE ?", queryPattern, queryPattern, queryPattern)
			} else {
				queryPattern := "%" + q + "%"
				session = session.Where("name LIKE ? OR note LIKE ? OR expression LIKE ?", queryPattern, queryPattern, queryPattern)
			}
		}
	}
	return session
}

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

func BuiltinMetricTypes(ctx *ctx.Context, lang, collector, query string) ([]string, error) {
	var types []string
	session := DB(ctx).Model(&BuiltinMetric{}).Where("updated_by != ?", SYSTEM)
	if lang != "" {
		session = session.Where("lang = ?", lang)
	}

	if collector != "" {
		session = session.Where("collector = ?", collector)
	}

	if query != "" {
		session = session.Where("typ like ?", "%"+query+"%")
	}

	err := session.Select("distinct(typ)").Pluck("typ", &types).Error
	return types, err
}

func BuiltinMetricCollectors(ctx *ctx.Context, lang, typ, query string) ([]string, error) {
	var collectors []string
	session := DB(ctx).Model(&BuiltinMetric{}).Where("updated_by != ?", SYSTEM)
	if lang != "" {
		session = session.Where("lang = ?", lang)
	}

	if typ != "" {
		session = session.Where("typ = ?", typ)
	}

	if query != "" {
		session = session.Where("collector like ?", "%"+query+"%")
	}

	err := session.Select("distinct(collector)").Pluck("collector", &collectors).Error
	return collectors, err
}

func BuiltinMetricBatchUpdateColumn(ctx *ctx.Context, col, old, new, updatedBy string) error {
	if old == new {
		return nil
	}
	return DB(ctx).Model(&BuiltinMetric{}).Where(fmt.Sprintf("%s = ?", col), old).Updates(map[string]interface{}{col: new, "updated_by": updatedBy}).Error
}
