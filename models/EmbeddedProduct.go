package models

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EmbeddedProduct struct {
	ID        uint64 `json:"id" gorm:"primaryKey"` // 主键
	Name      string `json:"name"`
	URL       string `json:"url"`
	IsPrivate bool   `json:"is_private"`
	TeamIDs   string `json:"-" gorm:"column:team_ids"` // 数据库存储为 JSON 字符串
	// 前端用的字段，GORM 忽略，自己处理序列化/反序列化
	TeamIDsJson []int64 `json:"team_ids" gorm:"-"` // 前端用的数组形式
	CreateAt    int64   `json:"create_at"`
	CreateBy    string  `json:"create_by"`
	UpdateAt    int64   `json:"update_at"`
	UpdateBy    string  `json:"update_by"`
}

func (e *EmbeddedProduct) TableName() string {
	return "embedded-product"
}

func (e *EmbeddedProduct) Verify() error {
	if e.Name == "" {
		return errors.New("Name is blank")
	}

	if str.Dangerous(e.Name) {
		return errors.New("Name has invalid characters")
	}

	return nil
}

func (e *EmbeddedProduct) BeforeSave(tx *gorm.DB) error {
	if len(e.TeamIDsJson) == 0 {
		e.TeamIDs = "[]"
		return nil
	}

	data, err := json.Marshal(e.TeamIDsJson)
	if err != nil {
		return err
	}
	e.TeamIDs = string(data)
	return nil
}

func (e *EmbeddedProduct) AfterFind(tx *gorm.DB) error {
	if e.TeamIDs == "" {
		e.TeamIDsJson = nil
		return nil
	}
	return json.Unmarshal([]byte(e.TeamIDs), &e.TeamIDsJson)
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

func ListEmbeddedProducts(ctx *ctx.Context) ([]*EmbeddedProduct, error) {
	var list []*EmbeddedProduct
	err := DB(ctx).Find(&list).Error
	return list, err
}

func GetEmbeddedProductByID(ctx *ctx.Context, id uint64) (*EmbeddedProduct, error) {
	var ep EmbeddedProduct
	err := DB(ctx).Where("id = ?", id).First(&ep).Error
	return &ep, err
}

func UpdateEmbeddedProduct(ctx *ctx.Context, ep *EmbeddedProduct, username string) error {
	if err := ep.Verify(); err != nil {
		return err
	}
	ep.UpdateAt = time.Now().Unix()
	ep.UpdateBy = username
	return DB(ctx).Save(ep).Error
}

func DeleteEmbeddedProduct(ctx *ctx.Context, id uint64) error {
	return DB(ctx).Where("id = ?", id).Delete(&EmbeddedProduct{}).Error
}
