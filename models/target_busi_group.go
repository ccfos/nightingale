package models

import (
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TargetBusiGroup struct {
	Id          int64  `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	TargetIdent string `json:"target_ident" gorm:"size:191;not null;index:idx_target_group,unique,priority:1"`
	GroupId     int64  `json:"group_id" gorm:"type:bigint;not null;index:idx_target_group,unique,priority:2"`
	UpdateAt    int64  `json:"update_at" gorm:"type:bigint;not null"`
}

func (t *TargetBusiGroup) TableName() string {
	return "target_busi_group"
}

func (t *TargetBusiGroup) TableOptions() string {
	return "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci"
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

// insertTargetBusiGroupsIgnoreDup 批量写入 target 与业务组的关联，已存在的跳过。
//
// 各方言的"忽略重复"写法不通用：MySQL 用 INSERT IGNORE，SQLite 用 INSERT OR IGNORE，
// PostgreSQL 用 ON CONFLICT DO NOTHING。达梦三种都不支持——它的 gorm dialector 只在
// 主键全部出现在插入列里时才把 OnConflict 翻译成 MERGE INTO，而这里的主键是自增的 id，
// 插入时为零值会被 gorm 省略，于是退化成裸 INSERT 撞上 (target_ident, group_id) 唯一索引。
// 所以达梦走先查后插：查出已有的组合，只插差集。
func insertTargetBusiGroupsIgnoreDup(db *gorm.DB, lst []TargetBusiGroup) error {
	if len(lst) == 0 {
		return nil
	}

	if db.Dialector.Name() == "dm" {
		idents := make([]string, 0, len(lst))
		seen := make(map[string]struct{}, len(lst))
		for _, item := range lst {
			if _, ok := seen[item.TargetIdent]; ok {
				continue
			}
			seen[item.TargetIdent] = struct{}{}
			idents = append(idents, item.TargetIdent)
		}

		var exists []TargetBusiGroup
		if err := db.Select("target_ident", "group_id").
			Where("target_ident in ?", idents).Find(&exists).Error; err != nil {
			return err
		}

		key := func(ident string, gid int64) string {
			return ident + "\x00" + strconv.FormatInt(gid, 10)
		}
		existing := make(map[string]struct{}, len(exists))
		for _, e := range exists {
			existing[key(e.TargetIdent, e.GroupId)] = struct{}{}
		}

		fresh := make([]TargetBusiGroup, 0, len(lst))
		for _, item := range lst {
			k := key(item.TargetIdent, item.GroupId)
			if _, ok := existing[k]; ok {
				continue
			}
			existing[k] = struct{}{} // 入参自身也可能有重复
			fresh = append(fresh, item)
		}
		if len(fresh) == 0 {
			return nil
		}
		return db.CreateInBatches(&fresh, 10).Error
	}

	var cl clause.Expression = clause.Insert{Modifier: "ignore"}
	switch db.Dialector.Name() {
	case "sqlite":
		cl = clause.Insert{Modifier: "or ignore"}
	case "postgres":
		cl = clause.OnConflict{DoNothing: true}
	}
	return db.Clauses(cl).CreateInBatches(&lst, 10).Error
}

func TargetBindBgids(ctx *ctx.Context, idents []string, bgids []int64, tags []string) error {
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
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := insertTargetBusiGroupsIgnoreDup(DB(ctx), lst); err != nil {
			return err
		}
		if targets, err := TargetsGetByIdents(ctx, idents); err != nil {
			return err
		} else if len(tags) > 0 {
			for _, t := range targets {
				if err := t.AddTags(ctx, tags); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func TargetUnbindBgids(ctx *ctx.Context, idents []string, bgids []int64) error {
	return DB(ctx).Where("target_ident in ? and group_id in ?",
		idents, bgids).Delete(&TargetBusiGroup{}).Error
}

func TargetDeleteBgids(tx *gorm.DB, idents []string) error {
	return tx.Where("target_ident in ?", idents).Delete(&TargetBusiGroup{}).Error
}

func TargetOverrideBgids(ctx *ctx.Context, idents []string, bgids []int64, tags []string) error {
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
		if err := insertTargetBusiGroupsIgnoreDup(tx, lst); err != nil {
			return err
		}
		if len(tags) == 0 {
			return nil
		}

		return tx.Model(Target{}).Where("ident IN ?", idents).Updates(map[string]interface{}{
			"tags": strings.Join(tags, " ") + " ", "update_at": updateAt}).Error
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

func TargetIndentsGetByBgids(ctx *ctx.Context, bgids []int64) ([]string, error) {
	var idents []string
	err := DB(ctx).Model(&TargetBusiGroup{}).
		Where("group_id IN ?", bgids).
		Distinct("target_ident").
		Pluck("target_ident", &idents).
		Error
	return idents, err
}
