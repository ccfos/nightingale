package models

import (
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

const (
	PublicAnonymous = 0
	PublicLogin     = 1
	PublicBusi      = 2
)

type Board struct {
	Id         int64   `json:"id" gorm:"primaryKey"`
	GroupId    int64   `json:"group_id"`
	Name       string  `json:"name"`
	Ident      string  `json:"ident"`
	Tags       string  `json:"tags"`
	CreateAt   int64   `json:"create_at"`
	CreateBy   string  `json:"create_by"`
	UpdateAt   int64   `json:"update_at"`
	UpdateBy   string  `json:"update_by"`
	Configs    string  `json:"configs" gorm:"-"`
	Public     int     `json:"public"`      // 0: false, 1: true
	PublicCate int     `json:"public_cate"` // 0: anonymous, 1: login, 2: busi
	Bgids      []int64 `json:"bgids" gorm:"-"`
	BuiltIn    int     `json:"built_in"` // 0: false, 1: true
	Hide       int     `json:"hide"`     // 0: false, 1: true
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

func (b *Board) Clone(operatorName string, newBgid int64, suffix string) *Board {
	clone := &Board{
		Name:     b.Name,
		Tags:     b.Tags,
		GroupId:  newBgid,
		CreateBy: operatorName,
		UpdateBy: operatorName,
	}

	if suffix != "" {
		clone.Name = clone.Name + " " + suffix
	}

	if b.Ident != "" {
		clone.Ident = uuid.NewString()
	}

	return clone
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

	cnt, err := Count(DB(ctx).Model(b).Where("name = ? and group_id = ?", b.Name, b.GroupId))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return errors.New("Name duplicate")
	}

	now := time.Now().Unix()
	b.CreateAt = now
	b.UpdateAt = now

	return Insert(ctx, b)
}

func (b *Board) AtomicAdd(c *ctx.Context, payload string) error {
	return DB(c).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{
			DB: tx,
		}

		if err := b.Add(tCtx); err != nil {
			return err
		}

		if payload != "" {
			if err := BoardPayloadSave(tCtx, b.Id, payload); err != nil {
				return err
			}
		}
		return nil
	})
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

func BoardGetsByBGIds(ctx *ctx.Context, gids []int64, query string) ([]Board, error) {
	session := DB(ctx)
	if len(gids) > 0 {
		session = session.Where("group_id in (?)", gids).Order("name")
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

func BoardGetsByBids(ctx *ctx.Context, bids []int64) ([]map[string]interface{}, error) {
	var boards []Board
	err := DB(ctx).Where("id IN ?", bids).Find(&boards).Error
	if err != nil {
		return nil, err
	}

	// 收集所有唯一的 group_id
	groupIDs := make([]int64, 0)
	groupIDSet := make(map[int64]struct{})
	for _, board := range boards {
		if _, exists := groupIDSet[board.GroupId]; !exists {
			groupIDs = append(groupIDs, board.GroupId)
			groupIDSet[board.GroupId] = struct{}{}
		}
	}

	// 一次性查询所有需要的 BusiGroup
	var busiGroups []BusiGroup
	err = DB(ctx).Where("id IN ?", groupIDs).Find(&busiGroups).Error
	if err != nil {
		return nil, err
	}

	// 创建 group_id 到 BusiGroup 的映射
	groupMap := make(map[int64]BusiGroup)
	for _, bg := range busiGroups {
		groupMap[bg.Id] = bg
	}

	result := make([]map[string]interface{}, 0, len(boards))
	for _, board := range boards {
		busiGroup, exists := groupMap[board.GroupId]
		if !exists {
			// 处理找不到对应 BusiGroup 的情况
			continue
		}

		item := map[string]interface{}{
			"busi_group_name": busiGroup.Name,
			"busi_group_id":   busiGroup.Id,
			"board_id":        board.Id,
			"board_name":      board.Name,
		}
		result = append(result, item)
	}

	return result, nil
}
