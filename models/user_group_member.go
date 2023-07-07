package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

type UserGroupMember struct {
	GroupId int64
	UserId  int64
}

func (UserGroupMember) TableName() string {
	return "user_group_member"
}

func (UserGroupMember) DB2FE() error {
	return nil
}
func MyGroupIds(ctx *ctx.Context, userId int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&UserGroupMember{}).Where("user_id=?", userId).Pluck("group_id", &ids).Error
	return ids, err
}

func MemberIds(ctx *ctx.Context, groupId int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&UserGroupMember{}).Where("group_id=?", groupId).Pluck("user_id", &ids).Error
	return ids, err
}

func UserGroupMemberCount(ctx *ctx.Context, where string, args ...interface{}) (int64, error) {
	return Count(DB(ctx).Model(&UserGroupMember{}).Where(where, args...))
}

func UserGroupMemberAdd(ctx *ctx.Context, groupId, userId int64) error {
	num, err := UserGroupMemberCount(ctx, "user_id=? and group_id=?", userId, groupId)
	if err != nil {
		return err
	}

	if num > 0 {
		// already exists
		return nil
	}

	obj := UserGroupMember{
		GroupId: groupId,
		UserId:  userId,
	}

	return Insert(ctx, obj)
}

func UserGroupMemberDel(ctx *ctx.Context, groupId int64, userIds []int64) error {
	if len(userIds) == 0 {
		return nil
	}

	return DB(ctx).Where("group_id = ? and user_id in ?", groupId, userIds).Delete(&UserGroupMember{}).Error
}

func UserGroupMemberGetAll(ctx *ctx.Context) ([]*UserGroupMember, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*UserGroupMember](ctx, "/v1/n9e/user-group-members")
		return lst, err
	}

	var lst []*UserGroupMember
	err := DB(ctx).Find(&lst).Error
	return lst, err
}

func UserGroupMemberStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=user_group_member")
		return s, err
	}
	// Updating statements are not allowed for user_group_member; changes can only be made through deletion and addition.
	session := DB(ctx).Model(&UserGroupMember{}).Select("count(*) as total", "max(id) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}
