package models

import (
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type BusiGroup struct {
	Id         int64                   `json:"id" gorm:"primaryKey"`
	Name       string                  `json:"name"`
	CreateAt   int64                   `json:"create_at"`
	CreateBy   string                  `json:"create_by"`
	UpdateAt   int64                   `json:"update_at"`
	UpdateBy   string                  `json:"update_by"`
	UserGroups []UserGroupWithPermFlag `json:"user_groups" gorm:"-"`
}

type UserGroupWithPermFlag struct {
	UserGroup *UserGroup `json:"user_group"`
	PermFlag  string     `json:"perm_flag"`
}

func (bg *BusiGroup) TableName() string {
	return "busi_group"
}

func (bg *BusiGroup) FillUserGroups() error {
	members, err := BusiGroupMemberGetsByBusiGroupId(bg.Id)
	if err != nil {
		return err
	}

	if len(members) == 0 {
		return nil
	}

	for i := 0; i < len(members); i++ {
		ug, err := UserGroupGetById(members[i].UserGroupId)
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

func BusiGroupGet(where string, args ...interface{}) (*BusiGroup, error) {
	var lst []*BusiGroup
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func BusiGroupGetById(id int64) (*BusiGroup, error) {
	return BusiGroupGet("id=?", id)
}

func BusiGroupExists(where string, args ...interface{}) (bool, error) {
	num, err := Count(DB().Model(&BusiGroup{}).Where(where, args...))
	return num > 0, err
}

func (bg *BusiGroup) Del() error {
	has, err := Exists(DB().Model(&AlertMute{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some alert mutes still in the BusiGroup")
	}

	has, err = Exists(DB().Model(&AlertSubscribe{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some alert subscribes still in the BusiGroup")
	}

	has, err = Exists(DB().Model(&Target{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some targets still in the BusiGroup")
	}

	has, err = Exists(DB().Model(&Dashboard{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some dashboards still in the BusiGroup")
	}

	has, err = Exists(DB().Model(&TaskTpl{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some recovery scripts still in the BusiGroup")
	}

	// hasCR, err := Exists(DB().Table("collect_rule").Where("group_id=?", bg.Id))
	// if err != nil {
	// 	return err
	// }

	// if hasCR {
	// 	return errors.New("Some collect rules still in the BusiGroup")
	// }

	has, err = Exists(DB().Model(&AlertRule{}).Where("group_id=?", bg.Id))
	if err != nil {
		return err
	}

	if has {
		return errors.New("Some alert rules still in the BusiGroup")
	}

	return DB().Transaction(func(tx *gorm.DB) error {
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

func (bg *BusiGroup) AddMembers(members []BusiGroupMember, username string) error {
	for i := 0; i < len(members); i++ {
		err := BusiGroupMemberAdd(members[i])
		if err != nil {
			return err
		}
	}

	return DB().Model(bg).Updates(map[string]interface{}{
		"update_at": time.Now().Unix(),
		"update_by": username,
	}).Error
}

func (bg *BusiGroup) DelMembers(members []BusiGroupMember, username string) error {
	for i := 0; i < len(members); i++ {
		err := BusiGroupMemberDel("busi_group_id = ? and user_group_id = ?", members[i].BusiGroupId, members[i].UserGroupId)
		if err != nil {
			return err
		}
	}

	return DB().Model(bg).Updates(map[string]interface{}{
		"update_at": time.Now().Unix(),
		"update_by": username,
	}).Error
}

func (bg *BusiGroup) Update(name string, updateBy string) error {
	if bg.Name == name {
		return nil
	}

	exists, err := BusiGroupExists("name = ? and id <> ?", name, bg.Id)
	if err != nil {
		return errors.WithMessage(err, "failed to count BusiGroup")
	}

	if exists {
		return errors.New("BusiGroup already exists")
	}

	return DB().Model(bg).Updates(map[string]interface{}{
		"name":      name,
		"update_at": time.Now().Unix(),
		"update_by": updateBy,
	}).Error
}

func BusiGroupAdd(name string, members []BusiGroupMember, creator string) error {
	exists, err := BusiGroupExists("name=?", name)
	if err != nil {
		return errors.WithMessage(err, "failed to count BusiGroup")
	}

	if exists {
		return errors.New("BusiGroup already exists")
	}

	count := len(members)
	for i := 0; i < count; i++ {
		ug, err := UserGroupGet("id=?", members[i].UserGroupId)
		if err != nil {
			return errors.WithMessage(err, "failed to get UserGroup")
		}

		if ug == nil {
			return errors.New("Some UserGroup id not exists")
		}
	}

	now := time.Now().Unix()
	obj := &BusiGroup{
		Name:     name,
		CreateAt: now,
		CreateBy: creator,
		UpdateAt: now,
		UpdateBy: creator,
	}

	return DB().Transaction(func(tx *gorm.DB) error {
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
