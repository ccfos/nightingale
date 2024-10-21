package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TargetBusiGroup struct {
	Id          int64  `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	TargetIdent string `json:"target_ident" gorm:"type:varchar(191);not null;index:idx_target_group,unique,priority:1"`
	GroupId     int64  `json:"group_id" gorm:"type:bigint;not null;index:idx_target_group,unique,priority:2"`
	UpdateAt    int64  `json:"update_at" gorm:"type:bigint;not null"`
}

func (t *TargetBusiGroup) TableName() string {
	return "target_busi_group"
}

func TargetBusiGroupsGetAll(ctx *ctx.Context) (map[string][]int64, error) {
	var lst []*TargetBusiGroup
	err := DB(ctx).Find(&lst).Error
	if err != nil {
		return nil, err
	}
	tgs := make(map[string][]int64)
	for _, tg := range lst {
		tgs[tg.TargetIdent] = append(tgs[tg.TargetIdent], tg.GroupId)
	}
	return tgs, nil
}

func TargetGroupIdsGetByIdent(ctx *ctx.Context, ident string) ([]int64, error) {
	var lst []*TargetBusiGroup
	err := DB(ctx).Where("target_ident = ?", ident).Find(&lst).Error
	if err != nil {
		return nil, err
	}
	groupIds := make([]int64, 0, len(lst))
	for _, tg := range lst {
		groupIds = append(groupIds, tg.GroupId)
	}
	return groupIds, nil
}

func TargetGroupIdsGetByIdents(ctx *ctx.Context, idents []string) ([]int64, error) {
	var groupIds []int64
	err := DB(ctx).Model(&TargetBusiGroup{}).
		Where("target_ident IN ?", idents).
		Distinct().
		Pluck("group_id", &groupIds).
		Error
	if err != nil {
		return nil, err
	}

	return groupIds, nil
}

func TargetBindBgids(ctx *ctx.Context, idents []string, bgids []int64) error {
	lst := make([]TargetBusiGroup, 0, len(bgids)*len(idents))
	updateAt := time.Now().Unix()
	for _, bgid := range bgids {
		for _, ident := range idents {
			cur := TargetBusiGroup{
				TargetIdent: ident,
				GroupId:     bgid,
				UpdateAt:    updateAt,
			}
			lst = append(lst, cur)
		}
	}

	var cl clause.Expression = clause.Insert{Modifier: "ignore"}
	switch DB(ctx).Dialector.Name() {
	case "sqlite":
		cl = clause.Insert{Modifier: "or ignore"}
	case "postgres":
		cl = clause.OnConflict{DoNothing: true}
	}
	return DB(ctx).Clauses(cl).CreateInBatches(&lst, 10).Error
}

func TargetUnbindBgids(ctx *ctx.Context, idents []string, bgids []int64) error {
	return DB(ctx).Where("target_ident in ? and group_id in ?",
		idents, bgids).Delete(&TargetBusiGroup{}).Error
}

func TargetDeleteBgids(ctx *ctx.Context, idents []string) error {
	return DB(ctx).Where("target_ident in ?", idents).Delete(&TargetBusiGroup{}).Error
}

func TargetOverrideBgids(ctx *ctx.Context, idents []string, bgids []int64) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		// 先删除旧的关联
		if err := tx.Where("target_ident IN ?", idents).Delete(&TargetBusiGroup{}).Error; err != nil {
			return err
		}

		// 准备新的关联数据
		lst := make([]TargetBusiGroup, 0, len(bgids)*len(idents))
		updateAt := time.Now().Unix()
		for _, ident := range idents {
			for _, bgid := range bgids {
				cur := TargetBusiGroup{
					TargetIdent: ident,
					GroupId:     bgid,
					UpdateAt:    updateAt,
				}
				lst = append(lst, cur)
			}
		}

		if len(lst) == 0 {
			return nil
		}

		// 添加新的关联
		var cl clause.Expression = clause.Insert{Modifier: "ignore"}
		switch tx.Dialector.Name() {
		case "sqlite":
			cl = clause.Insert{Modifier: "or ignore"}
		case "postgres":
			cl = clause.OnConflict{DoNothing: true}
		}
		return tx.Clauses(cl).CreateInBatches(&lst, 10).Error
	})
}

func SeparateTargetIdents(ctx *ctx.Context, idents []string) (existing, nonExisting []string, err error) {
	existingMap := make(map[string]bool)

	// 查询已存在的 idents 并直接填充 map
	err = DB(ctx).Model(&TargetBusiGroup{}).
		Where("target_ident IN ?", idents).
		Distinct().
		Pluck("target_ident", &existing).
		Error
	if err != nil {
		return nil, nil, err
	}

	for _, ident := range existing {
		existingMap[ident] = true
	}

	// 分离不存在的 idents
	for _, ident := range idents {
		if !existingMap[ident] {
			nonExisting = append(nonExisting, ident)
		}
	}

	return
}
