package models

import "github.com/ccfos/nightingale/v6/pkg/ctx"

type BusiGroupMember struct {
	BusiGroupId int64  `json:"busi_group_id"`
	UserGroupId int64  `json:"user_group_id"`
	PermFlag    string `json:"perm_flag"`
}

func (BusiGroupMember) TableName() string {
	return "busi_group_member"
}

func (bgm *BusiGroupMember) DB2FE() error {
	return nil
}

func BusiGroupIds(ctx *ctx.Context, userGroupIds []int64, permFlag ...string) ([]int64, error) {
	if len(userGroupIds) == 0 {
		return []int64{}, nil
	}

	session := DB(ctx).Model(&BusiGroupMember{}).Where("user_group_id in ?", userGroupIds)
	if len(permFlag) > 0 {
		session = session.Where("perm_flag=?", permFlag[0])
	}

	var ids []int64
	err := session.Pluck("busi_group_id", &ids).Error
	return ids, err
}

func UserGroupIdsOfBusiGroup(ctx *ctx.Context, busiGroupId int64, permFlag ...string) ([]int64, error) {
	session := DB(ctx).Model(&BusiGroupMember{}).Where("busi_group_id = ?", busiGroupId)
	if len(permFlag) > 0 {
		session = session.Where("perm_flag=?", permFlag[0])
	}

	var ids []int64
	err := session.Pluck("user_group_id", &ids).Error
	return ids, err
}

func BusiGroupMemberCount(ctx *ctx.Context, where string, args ...interface{}) (int64, error) {
	return Count(DB(ctx).Model(&BusiGroupMember{}).Where(where, args...))
}

func BusiGroupMemberAdd(ctx *ctx.Context, member BusiGroupMember) error {
	obj, err := BusiGroupMemberGet(ctx, "busi_group_id = ? and user_group_id = ?", member.BusiGroupId, member.UserGroupId)
	if err != nil {
		return err
	}

	if obj == nil {
		// insert
		return Insert(ctx, &BusiGroupMember{
			BusiGroupId: member.BusiGroupId,
			UserGroupId: member.UserGroupId,
			PermFlag:    member.PermFlag,
		})
	} else {
		// update
		if obj.PermFlag == member.PermFlag {
			return nil
		}

		return DB(ctx).Model(&BusiGroupMember{}).Where("busi_group_id = ? and user_group_id = ?", member.BusiGroupId, member.UserGroupId).Update("perm_flag", member.PermFlag).Error
	}
}

func BusiGroupMemberGet(ctx *ctx.Context, where string, args ...interface{}) (*BusiGroupMember, error) {
	var lst []*BusiGroupMember
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func BusiGroupMemberDel(ctx *ctx.Context, where string, args ...interface{}) error {
	return DB(ctx).Where(where, args...).Delete(&BusiGroupMember{}).Error
}

func BusiGroupMemberGets(ctx *ctx.Context, where string, args ...interface{}) ([]BusiGroupMember, error) {
	var lst []BusiGroupMember
	err := DB(ctx).Where(where, args...).Order("perm_flag").Find(&lst).Error
	return lst, err
}

func BusiGroupMemberGetsByBusiGroupId(ctx *ctx.Context, busiGroupId int64) ([]BusiGroupMember, error) {
	return BusiGroupMemberGets(ctx, "busi_group_id=?", busiGroupId)
}
