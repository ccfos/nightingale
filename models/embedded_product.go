package models

import (
	"encoding/json"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EmbeddedProduct struct {
	ID        int64   `json:"id" gorm:"primaryKey"` // 主键
	Name      string  `json:"name" gorm:"column:name;type:varchar(255)"`
	URL       string  `json:"url" gorm:"column:url;type:varchar(255)"`
	IsPrivate bool    `json:"is_private" gorm:"column:is_private;type:boolean"`
	TeamIDs   []int64 `json:"team_ids" gorm:"serializer:json"`
	CreateAt  int64   `json:"create_at" gorm:"column:create_at;not null;default:0"`
	CreateBy  string  `json:"create_by" gorm:"column:create_by;type:varchar(64);not null;default:''"`
	UpdateAt  int64   `json:"update_at" gorm:"column:update_at;not null;default:0"`
	UpdateBy  string  `json:"update_by" gorm:"column:update_by;type:varchar(64);not null;default:''"`
}

func (e *EmbeddedProduct) TableName() string {
	return "embedded_product"
}

func (e *EmbeddedProduct) AfterFind(tx *gorm.DB) (err error) {
	if e.TeamIDs == nil {
		e.TeamIDs = []int64{}
	}
	return nil
}

func (e *EmbeddedProduct) Verify() error {
	if e.Name == "" {
		return errors.New("Name is blank")
	}

	if str.Dangerous(e.Name) {
		return errors.New("Name has invalid characters")
	}

	if e.URL == "" {
		return errors.New("URL is blank")
	}

	if e.IsPrivate && len(e.TeamIDs) == 0 {
		return errors.New("TeamIDs is blank")
	}

	return nil
}

func AddEmbeddedProduct(ctx *ctx.Context, eps []EmbeddedProduct) error {
	now := time.Now().Unix()

	for i := range eps {
		if err := eps[i].Verify(); err != nil {
			return errors.Wrapf(err, "invalid entry %v", eps[i])
		}
		eps[i].CreateAt = now
		eps[i].UpdateAt = now
	}

	// 用主键做冲突判断，有冲突则更新（UPSERT）
	return DB(ctx).Clauses(clause.OnConflict{
		UpdateAll: true, // 冲突时更新所有字段
	}).Create(&eps).Error
}

func EmbeddedProductGets(ctx *ctx.Context) ([]*EmbeddedProduct, error) {
	var list []*EmbeddedProduct
	err := DB(ctx).Find(&list).Error
	return list, err
}

func GetEmbeddedProductByID(ctx *ctx.Context, id int64) (*EmbeddedProduct, error) {
	var ep EmbeddedProduct
	err := DB(ctx).Where("id = ?", id).First(&ep).Error
	return &ep, err
}

func UpdateEmbeddedProduct(ctx *ctx.Context, ep *EmbeddedProduct) error {
	if err := ep.Verify(); err != nil {
		return err
	}
	return DB(ctx).Save(ep).Error
}

func DeleteEmbeddedProduct(ctx *ctx.Context, id int64) error {
	return DB(ctx).Where("id = ?", id).Delete(&EmbeddedProduct{}).Error
}

func CanMigrateEP(ctx *ctx.Context) bool {
	var count int64
	err := DB(ctx).Model(&EmbeddedProduct{}).Count(&count).Error
	if err != nil {
		logger.Errorf("failed to get embedded-product table count, err:%v", err)
		return false
	}
	return count <= 0
}

func MigrateEP(ctx *ctx.Context) {
	var lst []string
	_ = DB(ctx).Model(&Configs{}).Where("ckey=?  and external=? ", "embedded-dashboards", 0).Pluck("cval", &lst).Error
	if len(lst) > 0 {
		var oldData []DashboardConfig
		if err := json.Unmarshal([]byte(lst[0]), &oldData); err != nil {
			return
		}

		if len(oldData) < 1 {
			return
		}

		now := time.Now().Unix()
		var newData []EmbeddedProduct
		for _, v := range oldData {
			newData = append(newData, EmbeddedProduct{
				Name:      v.Name,
				URL:       v.URL,
				IsPrivate: false,
				TeamIDs:   []int64{},
				CreateBy:  "system",
				CreateAt:  now,
				UpdateAt:  now,
				UpdateBy:  "system",
			})
		}
		err := DB(ctx).Create(&newData).Error
		if err != nil {
			logger.Errorf("failed to create embedded-product, err:%v", err)
		}
	}
}

type DashboardConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}
