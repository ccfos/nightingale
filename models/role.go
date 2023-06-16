package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
)

type Role struct {
	Id   int64  `json:"id" gorm:"primaryKey"`
	Name string `json:"name"`
	Note string `json:"note"`
}

func (Role) TableName() string {
	return "role"
}

func (r *Role) DB2FE() error {
	return nil
}

func RoleGets(ctx *ctx.Context, where string, args ...interface{}) ([]Role, error) {
	var objs []Role
	err := DB(ctx).Where(where, args...).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to query roles")
	}
	return objs, nil
}

func RoleGetsAll(ctx *ctx.Context) ([]Role, error) {
	return RoleGets(ctx, "")
}

// 增加角色
func (r *Role) Add(ctx *ctx.Context) error {
	role, err := RoleGet(ctx, "name = ?", r.Name)
	if err != nil {
		return errors.WithMessage(err, "failed to query user")
	}

	if role != nil {
		return errors.New("role name already exists")
	}

	return DB(ctx).Create(r).Error
}

// 删除角色
func (r *Role) Del(ctx *ctx.Context) error {
	return DB(ctx).Delete(r).Error
}

// 更新角色
func (ug *Role) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	return DB(ctx).Model(ug).Select(selectField, selectFields...).Updates(ug).Error
}

func RoleGet(ctx *ctx.Context, where string, args ...interface{}) (*Role, error) {
	var lst []*Role
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func RoleCount(ctx *ctx.Context, where string, args ...interface{}) (num int64, err error) {
	return Count(DB(ctx).Model(&Role{}).Where(where, args...))
}
