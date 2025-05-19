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
	ID          int64         `json:"id" gorm:"primaryKey;type:bigint;autoIncrement;comment:'unique identifier'"`
	UUID        int64         `json:"uuid" gorm:"type:bigint;not null;default:0;comment:'uuid'"`
	Collector   string        `json:"collector" gorm:"uniqueIndex:idx_collector_typ_name;type:varchar(191);not null;index:idx_collector,sort:asc;comment:'type of collector'"`
	Typ         string        `json:"typ" gorm:"uniqueIndex:idx_collector_typ_name;type:varchar(191);not null;index:idx_typ,sort:asc;comment:'type of metric'"`
	Name        string        `json:"name" gorm:"uniqueIndex:idx_collector_typ_name;type:varchar(191);not null;index:idx_builtinmetric_name,sort:asc;comment:'name of metric'"`
	Unit        string        `json:"unit" gorm:"type:varchar(191);not null;comment:'unit of metric'"`
	Note        string        `json:"note" gorm:"type:varchar(4096);not null;comment:'description of metric'"`
	Lang        string        `json:"lang" gorm:"uniqueIndex:idx_collector_typ_name;type:varchar(191);not null;default:'zh';index:idx_lang,sort:asc;comment:'language'"`
	Translation []Translation `json:"translation" gorm:"ttype:text;serializer:json;comment:'translation of metric'"`
	Enable      bool          `json:"enable" gorm:"not null;default:false;comment:'enable or disable metric'"`
	Expression  string        `json:"expression" gorm:"type:varchar(4096);not null;comment:'expression of metric'"`
	CreatedAt   int64         `json:"created_at" gorm:"type:bigint;not null;default:0;comment:'create time'"`
	CreatedBy   string        `json:"created_by" gorm:"type:varchar(191);not null;default:'';comment:'creator'"`
	UpdatedAt   int64         `json:"updated_at" gorm:"type:bigint;not null;default:0;comment:'update time'"`
	UpdatedBy   string        `json:"updated_by" gorm:"type:varchar(191);not null;default:'';comment:'updater'"`
}

type Translation struct {
	Lang string `json:"lang" gorm:"type:varchar(191);not null;default:'';comment:'language'"`
	Name string `json:"name" gorm:"type:varchar(191);not null;default:'';comment:'name of metric'"`
	Note string `json:"note" gorm:"type:varchar(4096);not null;default:'';comment:'description of metric'"`
}

func (bm *BuiltinMetric) TableName() string {
	return "builtin_metrics"
}

func (bm *BuiltinMetric) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
}

func (bm *BuiltinMetric) Verify() error {
	// Check english language is existed.
	hasEnglish := false
	for _, bml := range bm.Translation {
		if bml.Lang == "en_US" {
			hasEnglish = true
		}
	}
	if !hasEnglish {
		return errors.New("english language is required")
	}

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
	err := DB(ctx).Model(bm).Where("lang = ? and collector = ? and typ = ? and name = ?", bm.Lang, bm.Collector, bm.Typ, bm.Name).Count(&count).Error
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

	if bm.Lang != req.Lang && bm.Collector != req.Collector && bm.Typ != req.Typ && bm.Name != req.Name {
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
	req.Lang = bm.Lang
	req.UUID = bm.UUID

	return DB(ctx).Model(bm).Select("*").Updates(req).Error
}

func BuiltinMetricDels(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	return DB(ctx).Where("id in ?", ids).Delete(new(BuiltinMetric)).Error
}

func BuiltinMetricGets(ctx *ctx.Context, lang, collector, typ, query, unit string, limit, offset int) ([]*BuiltinMetric, error) {
	session := DB(ctx)
	session = builtinMetricQueryBuild(collector, session, typ, query, unit)
	var lst []*BuiltinMetric
	err := session.Limit(limit).Offset(offset).Order("collector asc, typ asc, name asc").Find(&lst).Error

	// Update current lst with current language, default is en_US
	for i := range lst {
		trans, err := getTranslationWithLanguage(lst[i], lang)
		if err != nil {
			return nil, err
		}

		lst[i].Name = trans.Name
		lst[i].Note = trans.Note
	}

	return lst, err
}

func getTranslationWithLanguage(bm *BuiltinMetric, lang string) (*Translation, error) {
	var defaultTranslation *Translation
	for _, t := range bm.Translation {
		if t.Lang == lang {
			return &t, nil
		}

		if t.Lang == "en_US" {
			defaultTranslation = &t
		}
	}

	if defaultTranslation != nil {
		return defaultTranslation, nil
	}

	return nil, errors.New("translation not found")
}

func BuiltinMetricCount(ctx *ctx.Context, collector, typ, query, unit string) (int64, error) {
	session := DB(ctx).Model(&BuiltinMetric{})
	session = builtinMetricQueryBuild(collector, session, typ, query, unit)

	var cnt int64
	err := session.Count(&cnt).Error

	return cnt, err
}

func builtinMetricQueryBuild(collector string, session *gorm.DB, typ string, query, unit string) *gorm.DB {
	// We need to filter out the enabled metrics
	session = session.Where("enable = ?", true)

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
	var typs []string
	session := DB(ctx).Model(&BuiltinMetric{})
	if lang != "" {
		session = session.Where("lang = ?", lang)
	}

	if collector != "" {
		session = session.Where("collector = ?", collector)
	}

	if query != "" {
		session = session.Where("typ like ?", "%"+query+"%")
	}

	err := session.Select("distinct(typ)").Pluck("typ", &typs).Error
	return typs, err
}

func BuiltinMetricCollectors(ctx *ctx.Context, lang, typ, query string) ([]string, error) {
	var collectors []string
	session := DB(ctx).Model(&BuiltinMetric{})
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
