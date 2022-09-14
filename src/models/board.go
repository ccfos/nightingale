package models

import (
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type Board struct {
	Id       int64  `json:"id" gorm:"primaryKey"`
	GroupId  int64  `json:"group_id"`
	Name     string `json:"name"`
	Tags     string `json:"tags"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
	Configs  string `json:"configs" gorm:"-"`
	Public   int    `json:"public"` // 0: false, 1: true
}

func (b *Board) TableName() string {
	return "board"
}

func (b *Board) Verify() error {
	if b.Name == "" {
		return errors.New("Name is blank")
	}

	if str.Dangerous(b.Name) {
		return errors.New("Name has invalid characters")
	}

	return nil
}

func (b *Board) Add() error {
	if err := b.Verify(); err != nil {
		return err
	}

	now := time.Now().Unix()
	b.CreateAt = now
	b.UpdateAt = now

	return Insert(b)
}

func (b *Board) Update(selectField interface{}, selectFields ...interface{}) error {
	if err := b.Verify(); err != nil {
		return err
	}

	return DB().Model(b).Select(selectField, selectFields...).Updates(b).Error
}

func (b *Board) Del() error {
	return DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id=?", b.Id).Delete(&BoardPayload{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", b.Id).Delete(&Board{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func BoardGetByID(id int64) (*Board, error) {
	var lst []*Board
	err := DB().Where("id = ?", id).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

// BoardGet for detail page
func BoardGet(where string, args ...interface{}) (*Board, error) {
	var lst []*Board
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	payload, err := BoardPayloadGet(lst[0].Id)
	if err != nil {
		return nil, err
	}

	lst[0].Configs = payload

	return lst[0], nil
}

func BoardCount(where string, args ...interface{}) (num int64, err error) {
	return Count(DB().Model(&Board{}).Where(where, args...))
}

func BoardExists(where string, args ...interface{}) (bool, error) {
	num, err := BoardCount(where, args...)
	return num > 0, err
}

// BoardGets for list page
func BoardGets(groupId int64, query string) ([]Board, error) {
	session := DB().Where("group_id=?", groupId).Order("name")

	arr := strings.Fields(query)
	if len(arr) > 0 {
		for i := 0; i < len(arr); i++ {
			if strings.HasPrefix(arr[i], "-") {
				q := "%" + arr[i][1:] + "%"
				session = session.Where("name not like ? and tags not like ?", q, q)
			} else {
				q := "%" + arr[i] + "%"
				session = session.Where("(name like ? or tags like ?)", q, q)
			}
		}
	}

	var objs []Board
	err := session.Find(&objs).Error
	return objs, err
}
