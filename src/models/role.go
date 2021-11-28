package models

import (
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

func RoleGets(where string, args ...interface{}) ([]Role, error) {
	var objs []Role
	err := DB().Where(where, args...).Order("name").Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to query roles")
	}
	return objs, nil
}

func RoleGetsAll() ([]Role, error) {
	return RoleGets("")
}
