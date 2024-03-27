package models

import (
	"errors"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// BuiltinMetric represents a metric along with its metadata.
type BuiltinMetric struct {
	ID         int64  `json:"id" gorm:"primaryKey"` // Unique identifier
	Collector  string `json:"collector"`            // Typ of collector (e.g., 'categraf', 'telegraf')
	Typ        string `json:"typ"`                  // Typ of metric (e.g., 'host', 'mysql', 'redis')
	Name       string `json:"name"`                 // Name of the metric
	Unit       string `json:"unit"`                 // Unit of the metric
	DescCN     string `json:"desc_cn"`              // Description in Chinese
	DescEN     string `json:"desc_en"`              // Description in English
	Expression string `json:"expression"`           // Expression for calculation
	CreatedAt  int64  `json:"created_at"`           // Creation timestamp (unix time)
	CreatedBy  string `json:"created_by"`           // Creator
	UpdatedAt  int64  `json:"updated_at"`           // Update timestamp (unix time)
	UpdatedBy  string `json:"updated_by"`           // Updater
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
	req.CreatedAt = bm.CreatedAt
	req.CreatedBy = bm.CreatedBy

	if err := req.Verify(); err != nil {
		return err
	}

	return DB(ctx).Model(bm).Select("*").Updates(req).Error
}

func BuiltinMetricDels(ctx *ctx.Context, ids []int64) error {
	for i := 0; i < len(ids); i++ {
		ret := DB(ctx).Where("id = ?", ids[i]).Delete(&BuiltinMetric{})
		if ret.Error != nil {
			return ret.Error
		}
	}
	return nil
}

func BuiltinMetricGetByID(ctx *ctx.Context, id int64) (*BuiltinMetric, error) {
	return BuiltinMetricGet(ctx, "id = ?", id)
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
		session = session.Where("name like ?", "%"+search+"%").Where("desc_cn like ?", "%"+search+"%").Where("desc_en like ?", "%"+search+"%")
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
