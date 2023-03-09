package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type BuiltinCate struct {
	Id     int64  `json:"id" gorm:"primaryKey"`
	Name   string `json:"name"`
	UserId int64  `json:"user_id"`
}

func (b *BuiltinCate) TableName() string {
	return "builtin_cate"
}

// 创建 builtin_cate
func (b *BuiltinCate) Create(c *ctx.Context) error {
	return Insert(c, b)
}

// 删除 builtin_cate
func BuiltinCateDelete(c *ctx.Context, name string, userId int64) error {
	return DB(c).Where("name=? and user_id=?", name, userId).Delete(&BuiltinCate{}).Error
}

// 根据 userId 获取 builtin_cate
func BuiltinCateGetByUserId(c *ctx.Context, userId int64) (map[string]BuiltinCate, error) {
	var builtinCates []BuiltinCate
	err := DB(c).Where("user_id=?", userId).Find(&builtinCates).Error
	var builtinCatesMap = make(map[string]BuiltinCate)
	for _, builtinCate := range builtinCates {
		builtinCatesMap[builtinCate.Name] = builtinCate
	}

	return builtinCatesMap, err
}
