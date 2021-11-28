package models

type UserGroupMember struct {
	GroupId int64
	UserId  int64
}

func (UserGroupMember) TableName() string {
	return "user_group_member"
}

func MyGroupIds(userId int64) ([]int64, error) {
	var ids []int64
	err := DB().Model(&UserGroupMember{}).Where("user_id=?", userId).Pluck("group_id", &ids).Error
	return ids, err
}

func MemberIds(groupId int64) ([]int64, error) {
	var ids []int64
	err := DB().Model(&UserGroupMember{}).Where("group_id=?", groupId).Pluck("user_id", &ids).Error
	return ids, err
}

func UserGroupMemberCount(where string, args ...interface{}) (int64, error) {
	return Count(DB().Model(&UserGroupMember{}).Where(where, args...))
}

func UserGroupMemberAdd(groupId, userId int64) error {
	num, err := UserGroupMemberCount("user_id=? and group_id=?", userId, groupId)
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

	return Insert(obj)
}

func UserGroupMemberDel(groupId int64, userIds []int64) error {
	if len(userIds) == 0 {
		return nil
	}

	return DB().Where("group_id = ? and user_id in ?", groupId, userIds).Delete(&UserGroupMember{}).Error
}

func UserGroupMemberGetAll() ([]UserGroupMember, error) {
	var lst []UserGroupMember
	err := DB().Find(&lst).Error
	return lst, err
}
