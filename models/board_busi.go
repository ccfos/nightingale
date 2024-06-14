package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm"
)

type BoardBusigroup struct {
	BusiGroupId int64 `json:"busi_group_id"`
	BoardId     int64 `json:"board_id"`
}

func (BoardBusigroup) TableName() string {
	return "board_busigroup"
}

func BoardBusigroupAdd(tx *gorm.DB, boardId int64, busiGroupIds []int64) error {
	if len(busiGroupIds) == 0 {
		return nil
	}

	for _, busiGroupId := range busiGroupIds {
		obj := BoardBusigroup{
			BusiGroupId: busiGroupId,
			BoardId:     boardId,
		}

		if err := tx.Create(obj).Error; err != nil {
			return err
		}
	}

	return nil
}

func BoardBusigroupUpdate(ctx *ctx.Context, boardId int64, busiGroupIds []int64) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("board_id=?", boardId).Delete(&BoardBusigroup{}).Error; err != nil {
			return err
		}

		if err := BoardBusigroupAdd(tx, boardId, busiGroupIds); err != nil {
			return err
		}
		return nil
	})
}

func BoardBusigroupDelByBoardId(ctx *ctx.Context, boardId int64) error {
	return DB(ctx).Where("board_id=?", boardId).Delete(&BoardBusigroup{}).Error
}

// BoardBusigroupCheck(rt.Ctx, board.Id, bgids)
func BoardBusigroupCheck(ctx *ctx.Context, boardId int64, busiGroupIds []int64) (bool, error) {
	count, err := Count(DB(ctx).Where("board_id=? and busi_group_id in (?)", boardId, busiGroupIds).Model(&BoardBusigroup{}))
	return count > 0, err
}

func BoardBusigroupGets(ctx *ctx.Context) ([]BoardBusigroup, error) {
	var objs []BoardBusigroup
	err := DB(ctx).Find(&objs).Error
	return objs, err
}

// get board ids by  busi group ids
func BoardIdsByBusiGroupIds(ctx *ctx.Context, busiGroupIds []int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&BoardBusigroup{}).Where("busi_group_id in (?)", busiGroupIds).Pluck("board_id", &ids).Error
	return ids, err
}
