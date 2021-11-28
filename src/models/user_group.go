package models

import (
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type UserGroup struct {
	Id       int64   `json:"id" gorm:"primaryKey"`
	Name     string  `json:"name"`
	Note     string  `json:"note"`
	CreateAt int64   `json:"create_at"`
	CreateBy string  `json:"create_by"`
	UpdateAt int64   `json:"update_at"`
	UpdateBy string  `json:"update_by"`
	UserIds  []int64 `json:"-" gorm:"-"`
}

func (ug *UserGroup) TableName() string {
	return "user_group"
}

func (ug *UserGroup) Verify() error {
	if str.Dangerous(ug.Name) {
		return errors.New("Name has invalid characters")
	}

	if str.Dangerous(ug.Note) {
		return errors.New("Note has invalid characters")
	}

	return nil
}

func (ug *UserGroup) Update(selectField interface{}, selectFields ...interface{}) error {
	if err := ug.Verify(); err != nil {
		return err
	}

	return DB().Model(ug).Select(selectField, selectFields...).Updates(ug).Error
}

func UserGroupCount(where string, args ...interface{}) (num int64, err error) {
	return Count(DB().Model(&UserGroup{}).Where(where, args...))
}

func (ug *UserGroup) Add() error {
	if err := ug.Verify(); err != nil {
		return err
	}

	num, err := UserGroupCount("name=?", ug.Name)
	if err != nil {
		return errors.WithMessage(err, "failed to count user-groups")
	}

	if num > 0 {
		return errors.New("UserGroup already exists")
	}

	now := time.Now().Unix()
	ug.CreateAt = now
	ug.UpdateAt = now
	return Insert(ug)
}

func (ug *UserGroup) Del() error {
	return DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id=?", ug.Id).Delete(&UserGroupMember{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", ug.Id).Delete(&UserGroup{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func GroupsOf(u *User) ([]UserGroup, error) {
	ids, err := MyGroupIds(u.Id)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get MyGroupIds")
	}

	session := DB().Where("create_by = ?", u.Username)
	if len(ids) > 0 {
		session = session.Or("id in ?", ids)
	}

	var lst []UserGroup
	err = session.Order("name").Find(&lst).Error
	return lst, err
}

func UserGroupGet(where string, args ...interface{}) (*UserGroup, error) {
	var lst []*UserGroup
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func UserGroupGetById(id int64) (*UserGroup, error) {
	return UserGroupGet("id = ?", id)
}

func UserGroupGetByIds(ids []int64) ([]UserGroup, error) {
	var lst []UserGroup
	if len(ids) == 0 {
		return lst, nil
	}

	err := DB().Where("id in ?", ids).Order("name").Find(&lst).Error
	return lst, err
}

func UserGroupGetAll() ([]*UserGroup, error) {
	var lst []*UserGroup
	err := DB().Find(&lst).Error
	return lst, err
}

func (ug *UserGroup) AddMembers(userIds []int64) error {
	count := len(userIds)
	for i := 0; i < count; i++ {
		user, err := UserGetById(userIds[i])
		if err != nil {
			return err
		}
		if user == nil {
			continue
		}
		err = UserGroupMemberAdd(ug.Id, user.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ug *UserGroup) DelMembers(userIds []int64) error {
	return UserGroupMemberDel(ug.Id, userIds)
}

func UserGroupStatistics() (*Statistics, error) {
	session := DB().Model(&UserGroup{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}
