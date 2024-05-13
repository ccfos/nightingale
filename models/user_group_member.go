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

func MyGroupIds(ctx *ctx.Context, userId int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Model(&UserGroupMember{}).Where("user_id=?", userId).Pluck("group_id", &ids).Error
	return ids, err
}

// my business group ids
func MyBusiGroupIds(ctx *ctx.Context, userId int64) ([]int64, error) {
	groupIds, err := MyGroupIds(ctx, userId)
	if err != nil {
		return []int64{}, err
	}

	return BusiGroupIds(ctx, groupIds)
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

// UserGroupMemberSync Sync group information, incrementally adding or overwriting deletes
func UserGroupMemberSync(ctx *ctx.Context, ldapGids []int64, userId int64, coverTeams bool) error {
	if len(ldapGids) == 0 {
		if coverTeams {
			// If the user is not in any group, delete all the groups that the user is currently in
			return DB(ctx).Where("user_id = ?", userId).Delete(&UserGroupMember{}).Error
		}

		return nil
	}

	// queries all the groups that the user is currently in
	allGrpIds, err := MyGroupIds(ctx, userId)
	if err != nil {
		return err
	}

	n := len(allGrpIds)
	allSet := slice2Set(allGrpIds)            // All the current groups Set
	toInsert := make([]UserGroupMember, 0, n) // Will be added

	// Prepare data for bulk insertion
	for _, gid := range ldapGids {
		if !allSet[gid] {
			// Add only groups where the user does not already exist
			toInsert = append(toInsert, UserGroupMember{GroupId: gid, UserId: userId})
			allSet[gid] = true
		}
	}
	// Bulk insert data
	if len(toInsert) == 0 {
		return nil
	}

	err = DB(ctx).CreateInBatches(toInsert, 10).Error
	if err != nil || !coverTeams {
		return err
	}

	// 需要将用户在 ldap 中没有, n9e 中有的团队删除
	ldapGidSet := slice2Set(ldapGids)  // The group to be processed Set
	toDeleteIds := make([]int64, 0, n) // The group ID to be deleted

	for _, gid := range allGrpIds {
		if !ldapGidSet[gid] {
			toDeleteIds = append(toDeleteIds, gid)
			ldapGidSet[gid] = true
		}
	}

	if len(toDeleteIds) == 0 {
		return nil
	}

	return DB(ctx).Where("user_id = ? AND group_id IN ?", userId, toDeleteIds).
		Delete(&UserGroupMember{}).Error
}

func UserGroupMemberSyncByUser(ctx *ctx.Context, user *User, coverTeams bool) error {
	if user == nil {
		return nil
	}

	return UserGroupMemberSync(ctx, user.TeamsLst, user.Id, coverTeams)
}

func slice2Set[T comparable](s []T) map[T]bool {
	m := make(map[T]bool, len(s))
	for _, item := range s {
		m[item] = true
	}

	return m
}
