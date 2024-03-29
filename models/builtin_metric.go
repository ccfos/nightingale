package models

import (
	"errors"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// BuiltinMetric represents a metric along with its metadata.
type BuiltinMetric struct {
    ID         uint64 `json:"id" gorm:"primaryKey;type:bigint;autoIncrement;comment:'unique identifier'"` // Unique identifier
    Collector  string `json:"collector" gorm:"type:varchar(191);not null;index:idx_collector,sort:asc;comment:'type of collector'"`            // Type of collector (e.g., 'categraf', 'telegraf')
    Typ        string `json:"typ" gorm:"type:varchar(191);not null;index:idx_typ,sort:asc;comment:'type of metric'"`                            // Type of metric (e.g., 'host', 'mysql', 'redis')
    Name       string `json:"name" gorm:"type:varchar(191);not null;index:idx_name,sort:asc;comment:'name of metric'"`                          // Name of the metric
    Unit       string `json:"unit" gorm:"type:varchar(191);not null;comment:'unit of metric'"`                                                   // Unit of the metric
    DescCN     string `json:"desc_cn" gorm:"type:varchar(4096);not null;comment:'description of metric in Chinese'"`                             // Description in Chinese
    DescEN     string `json:"desc_en" gorm:"type:varchar(4096);not null;comment:'description of metric in English'"`                            // Description in English
    Expression string `json:"expression" gorm:"type:varchar(4096);not null;comment:'expression of metric'"`                                     // Expression for calculation
    CreatedAt  int64  `json:"created_at" gorm:"type:bigint;not null;default:0;comment:'create time'"`                                           // Creation timestamp (unix time)
    CreatedBy  string `json:"created_by" gorm:"type:varchar(191);not null;default:'';comment:'creator'"`                                        // Creator
    UpdatedAt  int64  `json:"updated_at" gorm:"type:bigint;not null;default:0;comment:'update time'"`                                           // Update timestamp (unix time)
    UpdatedBy  string `json:"updated_by" gorm:"type:varchar(191);not null;default:'';comment:'updater'"`                                        // Updater
}

func (bm *BuiltinMetric) TableName() string {
	return "builtin_metrics"
}

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

func BuiltinMetricExists(ctx *ctx.Context, bm *BuiltinMetric) (bool, error) {
	var count int64
	err := DB(ctx).Model(bm).Where("collector = ? and typ = ? and name = ?", bm.Collector, bm.Typ, bm.Name).Count(&count).Error
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
	bm.CreatedBy = username
	return Insert(ctx, bm)
}

func (bm *BuiltinMetric) Update(ctx *ctx.Context, req BuiltinMetric) error {
	if err := req.Verify(); err != nil {
		return err
	}

	if bm.Collector != req.Collector && bm.Typ != req.Typ && bm.Name != req.Name {
		exists, err := BuiltinMetricExists(ctx, &req)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("builtin metric already exists")
		}
	}
	req.UpdatedAt = time.Now().Unix()

	return DB(ctx).Model(bm).Select("*").Updates(req).Error
}

func BuiltinMetricDels(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(BuiltinMetric)).Error
}

func BuiltinMetricGets(ctx *ctx.Context, collector, typ, search string, limit, offset int) ([]*BuiltinMetric, error) {
	session := DB(ctx)
	if collector != "" {
		session = session.Where("collector = ?", collector)
	}
	if typ != "" {
		session = session.Where("typ = ?", typ)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		session = session.Where("name LIKE ? OR desc_cn LIKE ? OR desc_en LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	var lst []*BuiltinMetric

	err := session.Limit(limit).Offset(offset).Find(&lst).Error

	return lst, err
}

func BuiltinMetricCount(ctx *ctx.Context, collector, typ, search string) (int64, error) {
	session := DB(ctx).Model(&BuiltinMetric{})
	if collector != "" {
		session = session.Where("collector = ?", collector)
	}
	if typ != "" {
		session = session.Where("typ = ?", typ)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		session = session.Where("name LIKE ? OR desc_cn LIKE ? OR desc_en LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	var cnt int64
	err := session.Count(&cnt).Error

	return cnt, err
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

func BuiltinMetricTypes(ctx *ctx.Context) ([]string, error) {
	var typs []string
	err := DB(ctx).Model(&BuiltinMetric{}).Select("distinct(typ)").Pluck("typ", &typs).Error
	return typs, err
}

func BuiltinMetricCollectors(ctx *ctx.Context) ([]string, error) {
	var collectors []string
	err := DB(ctx).Model(&BuiltinMetric{}).Select("distinct(collector)").Pluck("collector", &collectors).Error
	return collectors, err
}