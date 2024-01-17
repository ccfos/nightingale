package models

import (
	"github.com/ccfos/nightingale/v6/pkg/flashduty"
	"github.com/toolkits/pkg/logger"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

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

func (ug *UserGroup) DB2FE() error {
	return nil
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

func (ug *UserGroup) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if err := ug.Verify(); err != nil {
		return err
	}

	return DB(ctx).Model(ug).Select(selectField, selectFields...).Updates(ug).Error
}

func UserGroupCount(ctx *ctx.Context, where string, args ...interface{}) (num int64, err error) {
	return Count(DB(ctx).Model(&UserGroup{}).Where(where, args...))
}

func (ug *UserGroup) Add(ctx *ctx.Context) error {
	if err := ug.Verify(); err != nil {
		return err
	}

	num, err := UserGroupCount(ctx, "name=?", ug.Name)
	if err != nil {
		return errors.WithMessage(err, "failed to count user-groups")
	}

	if num > 0 {
		return errors.New("Team already exists")
	}

	now := time.Now().Unix()
	ug.CreateAt = now
	ug.UpdateAt = now
	return Insert(ctx, ug)
}

func (ug *UserGroup) Del(ctx *ctx.Context) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id=?", ug.Id).Delete(&UserGroupMember{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", ug.Id).Delete(&UserGroup{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func UserGroupGet(ctx *ctx.Context, where string, args ...interface{}) (*UserGroup, error) {
	var lst []*UserGroup
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func UserGroupGetById(ctx *ctx.Context, id int64) (*UserGroup, error) {
	return UserGroupGet(ctx, "id = ?", id)
}

func UserGroupGetByIds(ctx *ctx.Context, ids []int64) ([]UserGroup, error) {
	var lst []UserGroup
	if len(ids) == 0 {
		return lst, nil
	}

	err := DB(ctx).Where("id in ?", ids).Order("name").Find(&lst).Error
	return lst, err
}

func UserGroupGetAll(ctx *ctx.Context) ([]*UserGroup, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*UserGroup](ctx, "/v1/n9e/user-groups")
		return lst, err
	}

	var lst []*UserGroup
	err := DB(ctx).Find(&lst).Error
	return lst, err
}

func (ug *UserGroup) AddMembers(ctx *ctx.Context, userIds []int64) error {
	count := len(userIds)
	for i := 0; i < count; i++ {
		user, err := UserGetById(ctx, userIds[i])
		if err != nil {
			return err
		}
		if user == nil {
			continue
		}
		err = UserGroupMemberAdd(ctx, ug.Id, user.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ug *UserGroup) DelMembers(ctx *ctx.Context, userIds []int64) error {
	return UserGroupMemberDel(ctx, ug.Id, userIds)
}

func UserGroupStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=user_group")
		return s, err
	}

	session := DB(ctx).Model(&UserGroup{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func (ug *UserGroup) SyncAddToFlashDuty(ctx *ctx.Context) error {
	return ug.syncTeamMemberToFlashDuty(ctx)
}

func (ug *UserGroup) SyncPutToFlashDuty(ctx *ctx.Context, oldTeamName string) error {
	if err := ug.syncTeamMemberToFlashDuty(ctx); err != nil {
		return err
	}

	if oldTeamName != ug.Name {
		oldUg := &UserGroup{
			Name: oldTeamName,
		}
		if err := oldUg.SyncDelToFlashDuty(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (ug *UserGroup) SyncDelToFlashDuty(ctx *ctx.Context) error {
	appKey, err := ConfigsGetFlashDutyAppKey(ctx)
	if err != nil {
		return err
	}
	if appKey == "" {
		return nil
	}
	fdt := flashduty.Team{
		TeamName: ug.Name,
	}
	err = fdt.DelTeam(appKey)
	return err
}

func (ug *UserGroup) SyncMembersPutToFlashDuty(ctx *ctx.Context) error {
	return ug.syncTeamMemberToFlashDuty(ctx)
}

func (ug *UserGroup) syncTeamMemberToFlashDuty(ctx *ctx.Context) error {
	appKey, err := ConfigsGetFlashDutyAppKey(ctx)
	if err != nil {
		return err
	}
	if appKey == "" {
		logger.Warningf("The app_key is empty and failed to sync to flashduty")
		return nil
	}

	uids, err := MemberIds(ctx, ug.Id)
	if err != nil {
		return err
	}
	users, err := UserGetsByIds(ctx, uids)
	if err != nil {
		return err
	}
	err = ug.addMemberToFDTeam(appKey, users)

	return err
}

func (ug *UserGroup) addMemberToFDTeam(appKey string, users []User) error {
	fdusers := usersToFdUsers(users)
	if err := flashduty.AddUsers(appKey, fdusers); err != nil {
		return err
	}

	emails := make([]string, 0)
	phones := make([]string, 0)
	for _, user := range fdusers {
		if user.Email != "" {
			emails = append(emails, user.Email)
		} else if user.Phone != "" {
			phones = append(phones, user.Phone)
		} else {
			logger.Warningf("The user %s has no email and phone, and failed to sync to flashduty's team", user.MemberName)
		}
	}

	fdt := flashduty.Team{
		TeamName: ug.Name,
		Emails:   emails,
		Phones:   phones,
	}
	err := fdt.UpdateTeam(appKey)
	return err
}
