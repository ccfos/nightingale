package models

import (
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type UserGroup struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Note     string `json:"note"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

func (ug *UserGroup) TableName() string {
	return "user_group"
}

func (ug *UserGroup) Validate() error {
	if str.Dangerous(ug.Name) {
		return _e("Group name has invalid characters")
	}

	if str.Dangerous(ug.Note) {
		return _e("Group note has invalid characters")
	}

	return nil
}

func (ug *UserGroup) Add() error {
	if err := ug.Validate(); err != nil {
		return err
	}

	num, err := UserGroupCount("name=?", ug.Name)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("UserGroup %s already exists", ug.Name)
	}

	now := time.Now().Unix()
	ug.CreateAt = now
	ug.UpdateAt = now
	return DBInsertOne(ug)
}

func (ug *UserGroup) Update(cols ...string) error {
	if err := ug.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", ug.Id).Cols(cols...).Update(ug)
	if err != nil {
		logger.Errorf("mysql.error: update user_group(id=%d) fail: %v", ug.Id, err)
		return internalServerError
	}

	return nil
}

func UserGroupTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("name like ? or note like ?", q, q).Count(new(UserGroup))
	} else {
		num, err = DB.Count(new(UserGroup))
	}

	if err != nil {
		logger.Errorf("mysql.error: count user_group fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func UserGroupCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(UserGroup))
	if err != nil {
		logger.Errorf("mysql.error: count user_group fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func UserGroupGets(query string, limit, offset int) ([]UserGroup, error) {
	session := DB.Limit(limit, offset).OrderBy("name")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("name like ? or note like ?", q, q)
	}

	var objs []UserGroup
	err := session.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query user_group fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []UserGroup{}, nil
	}

	return objs, nil
}

func UserGroupGetsByIdsStr(ids []string) ([]UserGroup, error) {
	if len(ids) == 0 {
		return []UserGroup{}, nil
	}

	var objs []UserGroup

	err := DB.Where("id in (" + strings.Join(ids, ",") + ")").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: UserGroupGetsByIds fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []UserGroup{}, nil
	}

	return objs, nil
}

func UserGroupGet(where string, args ...interface{}) (*UserGroup, error) {
	var obj UserGroup
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query user_group(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (ug *UserGroup) MemberIds() ([]int64, error) {
	var ids []int64
	err := DB.Table(new(UserGroupMember)).Select("user_id").Where("group_id=?", ug.Id).Find(&ids)
	if err != nil {
		logger.Errorf("mysql.error: query user_group_member fail: %v", err)
		return ids, internalServerError
	}

	if len(ids) == 0 {
		return []int64{}, nil
	}

	return ids, nil
}

func (ug *UserGroup) AddMembers(userIds []int64) error {
	count := len(userIds)
	for i := 0; i < count; i++ {
		user, err := UserGetById(userIds[i])
		if err != nil {
			return err
		}
		if user == nil {
			continue
		}
		err = UserGroupMemberAdd(ug.Id, user.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ug *UserGroup) DelMembers(userIds []int64) error {
	return UserGroupMemberDel(ug.Id, userIds)
}

func (ug *UserGroup) Del() error {
	session := DB.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM user_group_member WHERE group_id=?", ug.Id); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM user_group WHERE id=?", ug.Id); err != nil {
		return err
	}

	return session.Commit()
}
