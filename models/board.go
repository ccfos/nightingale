package models

import (
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type Board struct {
	Id       int64  `json:"id" gorm:"primaryKey"`
	GroupId  int64  `json:"group_id"`
	Name     string `json:"name"`
	Ident    string `json:"ident"`
	Tags     string `json:"tags"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
	Configs  string `json:"configs" gorm:"-"`
	Public   int    `json:"public"`   // 0: false, 1: true
	BuiltIn  int    `json:"built_in"` // 0: false, 1: true
	Hide     int    `json:"hide"`     // 0: false, 1: true
}

func (b *Board) TableName() string {
	return "board"
}

func (b *Board) DB2FE() error {
	return nil
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

func (b *Board) CanRenameIdent(ctx *ctx.Context, ident string) (bool, error) {
	if ident == "" {
		return true, nil
	}

	cnt, err := Count(DB(ctx).Model(b).Where("ident=? and id <> ?", ident, b.Id))
	if err != nil {
		return false, err
	}

	return cnt == 0, nil
}

func (b *Board) Add(ctx *ctx.Context) error {
	if err := b.Verify(); err != nil {
		return err
	}

	if b.Ident != "" {
		// ident duplicate check
		cnt, err := Count(DB(ctx).Model(b).Where("ident=?", b.Ident))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return errors.New("Ident duplicate")
		}
	}

	now := time.Now().Unix()
	b.CreateAt = now
	b.UpdateAt = now

	return Insert(ctx, b)
}

func (b *Board) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if err := b.Verify(); err != nil {
		return err
	}

	return DB(ctx).Model(b).Select(selectField, selectFields...).Updates(b).Error
}

func (b *Board) Del(ctx *ctx.Context) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id=?", b.Id).Delete(&BoardPayload{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", b.Id).Delete(&Board{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func BoardGetByID(ctx *ctx.Context, id int64) (*Board, error) {
	var lst []*Board
	err := DB(ctx).Where("id = ?", id).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

// BoardGet for detail page
func BoardGet(ctx *ctx.Context, where string, args ...interface{}) (*Board, error) {
	var lst []*Board
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	payload, err := BoardPayloadGet(ctx, lst[0].Id)
	if err != nil {
		return nil, err
	}

	lst[0].Configs = payload

	return lst[0], nil
}

func BoardCount(ctx *ctx.Context, where string, args ...interface{}) (num int64, err error) {
	return Count(DB(ctx).Model(&Board{}).Where(where, args...))
}

func BoardExists(ctx *ctx.Context, where string, args ...interface{}) (bool, error) {
	num, err := BoardCount(ctx, where, args...)
	return num > 0, err
}

// BoardGets for list page
func BoardGetsByGroupId(ctx *ctx.Context, groupId int64, query string) ([]Board, error) {
	session := DB(ctx).Where("group_id=?", groupId).Order("name")

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

func BoardGets(ctx *ctx.Context, query, where string, args ...interface{}) ([]Board, error) {
	session := DB(ctx).Order("name")
	if where != "" {
		session = session.Where(where, args...)
	}

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

func BoardSetHide(ctx *ctx.Context, ids []int64) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Board{}).Where("built_in = 1").Update("hide", 0).Error; err != nil {
			return err
		}

		if err := tx.Model(&Board{}).Where("id in (?) and built_in = 1", ids).Update("hide", 1).Error; err != nil {
			return err
		}
		return nil
	})
}
