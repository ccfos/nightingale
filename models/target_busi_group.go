package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
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
