package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type BusiGroup struct {
	Id          int64                   `json:"id" gorm:"primaryKey"`
	Name        string                  `json:"name"`
	LabelEnable int                     `json:"label_enable"`
	LabelValue  string                  `json:"label_value"`
	CreateAt    int64                   `json:"create_at"`
	CreateBy    string                  `json:"create_by"`
	UpdateAt    int64                   `json:"update_at"`
	UpdateBy    string                  `json:"update_by"`
	UserGroups  []UserGroupWithPermFlag `json:"user_groups" gorm:"-"`
	DB          *gorm.DB                `json:"-" gorm:"-"`
}

func New(db *gorm.DB) *BusiGroup {
	return &BusiGroup{
		DB: db,
	}
}

type UserGroupWithPermFlag struct {
	UserGroup *UserGroup `json:"user_group"`
	PermFlag  string     `json:"perm_flag"`
}

func (bg *BusiGroup) TableName() string {
	return "busi_group"
}

func (bg *BusiGroup) FillUserGroups(ctx *ctx.Context) error {
	members, err := BusiGroupMemberGetsByBusiGroupId(ctx, bg.Id)
	if err != nil {
		return err
	}

	if len(members) == 0 {
		return nil
	}

	for i := 0; i < len(members); i++ {
		ug, err := UserGroupGetById(ctx, members[i].UserGroupId)
		if err != nil {
			return err
		}
		bg.UserGroups = append(bg.UserGroups, UserGroupWithPermFlag{
			UserGroup: ug,
			PermFlag:  members[i].PermFlag,
		})
	}

	return nil
}

func BusiGroupGetMap(ctx *ctx.Context) (map[int64]*BusiGroup, error) {
	var lst []*BusiGroup
	err := DB(ctx).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	ret := make(map[int64]*BusiGroup)
	for i := 0; i < len(lst); i++ {
		ret[lst[i].Id] = lst[i]
	}

	return ret, nil
}

func BusiGroupGet(ctx *ctx.Context, where string, args ...interface{}) (*BusiGroup, error) {
	var lst []*BusiGroup
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func BusiGroupGetById(ctx *ctx.Context, id int64) (*BusiGroup, error) {
	return BusiGroupGet(ctx, "id=?", id)
}

func BusiGroupExists(ctx *ctx.Context, where string, args ...interface{}) (bool, error) {
	num, err := Count(DB(ctx).Model(&BusiGroup{}).Where(where, args...))
	return num > 0, err
}

func (bg *BusiGroup) Del(ctx *ctx.Context) error {
	has, err := Exists(DB(ctx).Model(&AlertMute{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some alert mutes still in the BusiGroup")
	}

	has, err = Exists(DB(ctx).Model(&AlertSubscribe{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some alert subscribes still in the BusiGroup")
	}

	has, err = Exists(DB(ctx).Model(&Target{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some targets still in the BusiGroup")
	}

	has, err = Exists(DB(ctx).Model(&Board{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some dashboards still in the BusiGroup")
	}

	has, err = Exists(DB(ctx).Model(&TaskTpl{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some recovery scripts still in the BusiGroup")
	}

	// hasCR, err := Exists(DB(ctx).Table("collect_rule").Where("group_id=?", bg.Id))
	// if err != nil {
	// 	return err
	// }

	// if hasCR {
	// 	return errors.New("Some collect rules still in the BusiGroup")
	// }

	has, err = Exists(DB(ctx).Model(&AlertRule{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some alert rules still in the BusiGroup")
	}

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("busi_group_id=?", bg.Id).Delete(&BusiGroupMember{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", bg.Id).Delete(&BusiGroup{}).Error; err != nil {
			return err
		}

		// 这个需要好好斟酌一下，删掉BG，对应的活跃告警事件也一并删除
		// BG都删了，说明下面已经没有告警规则了，说明这些活跃告警永远都不会恢复了
		// 而且这些活跃告警已经没人关心了，既然是没人关心的，删了吧
		if err := tx.Where("group_id=?", bg.Id).Delete(&AlertCurEvent{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (bg *BusiGroup) AddMembers(ctx *ctx.Context, members []BusiGroupMember, username string) error {
	for i := 0; i < len(members); i++ {
		err := BusiGroupMemberAdd(ctx, members[i])
		if err != nil {
			return err
		}
	}

	return DB(ctx).Model(bg).Updates(map[string]interface{}{
		"update_at": time.Now().Unix(),
		"update_by": username,
	}).Error
}

func (bg *BusiGroup) DelMembers(ctx *ctx.Context, members []BusiGroupMember, username string) error {
	for i := 0; i < len(members); i++ {
		num, err := BusiGroupMemberCount(ctx, "busi_group_id = ? and user_group_id <> ?", members[i].BusiGroupId, members[i].UserGroupId)
		if err != nil {
			return err
		}

		if num == 0 {
			// 说明这是最后一个user-group，如果再删了，就没人可以管理这个busi-group了
			return fmt.Errorf("the business group must retain at least one team")
		}

		err = BusiGroupMemberDel(ctx, "busi_group_id = ? and user_group_id = ?", members[i].BusiGroupId, members[i].UserGroupId)
		if err != nil {
			return err
		}
	}

	return DB(ctx).Model(bg).Updates(map[string]interface{}{
		"update_at": time.Now().Unix(),
		"update_by": username,
	}).Error
}

func (bg *BusiGroup) Update(ctx *ctx.Context, name string, labelEnable int, labelValue string, updateBy string) error {
	if bg.Name == name && bg.LabelEnable == labelEnable && bg.LabelValue == labelValue {
		return nil
	}

	exists, err := BusiGroupExists(ctx, "name = ? and id <> ?", name, bg.Id)
	if err != nil {
		return errors.WithMessage(err, "failed to count BusiGroup")
	}

	if exists {
		return errors.New("BusiGroup already exists")
	}

	if labelEnable == 1 {
		exists, err = BusiGroupExists(ctx, "label_enable = 1 and label_value = ? and id <> ?", labelValue, bg.Id)
		if err != nil {
			return errors.WithMessage(err, "failed to count BusiGroup")
		}

		if exists {
			return errors.New("BusiGroup already exists")
		}
	} else {
		labelValue = ""
	}

	return DB(ctx).Model(bg).Updates(map[string]interface{}{
		"name":         name,
		"label_enable": labelEnable,
		"label_value":  labelValue,
		"update_at":    time.Now().Unix(),
		"update_by":    updateBy,
	}).Error
}

func BusiGroupAdd(ctx *ctx.Context, name string, labelEnable int, labelValue string, members []BusiGroupMember, creator string) error {
	exists, err := BusiGroupExists(ctx, "name=?", name)
	if err != nil {
		return errors.WithMessage(err, "failed to count BusiGroup")
	}

	if exists {
		return errors.New("BusiGroup already exists")
	}

	if labelEnable == 1 {
		exists, err = BusiGroupExists(ctx, "label_enable = 1 and label_value = ?", labelValue)
		if err != nil {
			return errors.WithMessage(err, "failed to count BusiGroup")
		}

		if exists {
			return errors.New("BusiGroup already exists")
		}
	} else {
		labelValue = ""
	}

	count := len(members)
	for i := 0; i < count; i++ {
		ug, err := UserGroupGet(ctx, "id=?", members[i].UserGroupId)
		if err != nil {
			return errors.WithMessage(err, "failed to get UserGroup")
		}

		if ug == nil {
			return errors.New("Some UserGroup id not exists")
		}
	}

	now := time.Now().Unix()
	obj := &BusiGroup{
		Name:        name,
		LabelEnable: labelEnable,
		LabelValue:  labelValue,
		CreateAt:    now,
		CreateBy:    creator,
		UpdateAt:    now,
		UpdateBy:    creator,
	}

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(obj).Error; err != nil {
			return err
		}

		for i := 0; i < len(members); i++ {
			if err := tx.Create(&BusiGroupMember{
				BusiGroupId: obj.Id,
				UserGroupId: members[i].UserGroupId,
				PermFlag:    members[i].PermFlag,
			}).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func BusiGroupStatistics(ctx *ctx.Context) (*Statistics, error) {
	session := DB(ctx).Model(&BusiGroup{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}
