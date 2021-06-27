package models

import (
	"github.com/toolkits/pkg/logger"
)

type UserGroupMember struct {
	GroupId int64
	UserId  int64
}

func (UserGroupMember) TableName() string {
	return "user_group_member"
}

func UserGroupMemberGetAll() ([]UserGroupMember, error) {
	var objs []UserGroupMember
	err := DB.Find(&objs)
	return objs, err
}

func UserGroupMemberCount(where string, args ...interface{}) (int64, error) {
	num, err := DB.Where(where, args...).Count(new(UserGroupMember))
	if err != nil {
		logger.Errorf("mysql.error: count user_group_member(where=%s, args=%+v) fail: %v", where, args, err)
		return 0, internalServerError
	}
	return num, nil
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

	return DBInsertOne(obj)
}

func UserGroupMemberDel(groupId int64, userIds []int64) error {
	if len(userIds) == 0 {
		return nil
	}

	_, err := DB.Where("group_id=?", groupId).In("user_id", userIds).Delete(new(UserGroupMember))
	if err != nil {
		logger.Errorf("mysql.error: delete user_group_member fail: %v", err)
		return internalServerError
	}

	return nil
}
