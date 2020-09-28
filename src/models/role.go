package models

import (
	"fmt"
	"strings"

	"xorm.io/xorm"

	"github.com/toolkits/pkg/str"
)

type Role struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
	Cate string `json:"cate"`
}

func (r *Role) CheckFields() error {
	if len(r.Name) > 32 {
		return fmt.Errorf("name too long")
	}

	if len(r.Note) > 128 {
		return fmt.Errorf("note too long")
	}

	if r.Cate != "global" && r.Cate != "local" {
		return fmt.Errorf("cate invalid")
	}

	if str.Dangerous(r.Name) {
		return fmt.Errorf("name dangerous")
	}

	if str.Dangerous(r.Note) {
		return fmt.Errorf("note dangerous")
	}

	if strings.ContainsAny(r.Name, ".%/") {
		return fmt.Errorf("name invalid")
	}

	return nil
}

func RoleGetByIds(ids []int64) (roles []Role, err error) {
	if len(ids) == 0 {
		return roles, nil
	}

	err = DB["rdb"].In("id", ids).OrderBy("name").Find(&roles)
	return roles, err
}

func RoleFind(cate string) ([]Role, error) {
	var objs []Role

	session := DB["rdb"].OrderBy("name")
	if cate != "" {
		session = session.Where("cate=?", cate)
	}

	err := session.Find(&objs)
	return objs, err
}

// RoleMap key: role_id, value: role_name
func RoleMap(cate string) (map[int64]string, error) {
	roles, err := RoleFind(cate)
	if err != nil {
		return nil, err
	}

	count := len(roles)
	m := make(map[int64]string, count)
	for i := 0; i < count; i++ {
		m[roles[i].Id] = roles[i].Name
	}

	return m, nil
}

func (r *Role) Save(ops []string) error {
	if err := r.CheckFields(); err != nil {
		return err
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	cnt, err := session.Where("name=?", r.Name).Count(new(Role))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("role[%s] already exists", r.Name)
	}

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Insert(r); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(ops); i++ {
		if err = roleOpAdd(session, r.Id, ops[i]); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func roleOpAdd(session *xorm.Session, rid int64, op string) error {
	var link RoleOperation
	has, err := session.Where("role_id=? and operation=?", rid, op).Get(&link)
	if err != nil {
		return err
	}

	if has {
		return nil
	}

	_, err = session.Insert(&RoleOperation{
		RoleId:    rid,
		Operation: op,
	})

	return err
}

func RoleGet(where string, args ...interface{}) (*Role, error) {
	var obj Role
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (r *Role) Modify(name, note, cate string, ops []string) error {
	if r.Name != name {
		cnt, err := DB["rdb"].Where("name = ? and id <> ?", name, r.Id).Count(new(Role))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return fmt.Errorf("role[%s] already exists", name)
		}
	}

	tolocal := false
	if r.Cate != cate && cate == "local" {
		tolocal = true
	}

	r.Name = name
	r.Note = note
	r.Cate = cate

	if err := r.CheckFields(); err != nil {
		return err
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Where("id=?", r.Id).Cols("name", "note", "cate").Update(r); err != nil {
		session.Rollback()
		return err
	}

	if tolocal {
		// 原来作为全局角色绑定的人员，就没有用了
		if _, err := session.Exec("DELETE FROM role_global_user WHERE role_id=?", r.Id); err != nil {
			session.Rollback()
			return err
		}
	}

	if _, err := session.Exec("DELETE FROM role_operation WHERE role_id=?", r.Id); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(ops); i++ {
		if err := roleOpAdd(session, r.Id, ops[i]); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func (r *Role) Del() error {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM role_operation WHERE role_id=?", r.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM role_global_user WHERE role_id=?", r.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM node_role WHERE role_id=?", r.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM role WHERE id=?", r.Id); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (r *Role) BindUsers(userIds []int64) error {
	ids, err := safeUserIds(userIds)
	if err != nil {
		return err
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	for i := 0; i < len(ids); i++ {
		cnt, err := session.Where("role_id=? and user_id=?", r.Id, ids[i]).Count(new(RoleGlobalUser))
		if err != nil {
			return err
		}

		if cnt > 0 {
			continue
		}

		_, err = session.Insert(RoleGlobalUser{RoleId: r.Id, UserId: ids[i]})
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Role) UnbindUsers(ids []int64) error {
	if ids == nil || len(ids) == 0 {
		return nil
	}

	_, err := DB["rdb"].Where("role_id=?", r.Id).In("user_id", ids).Delete(new(RoleGlobalUser))
	return err
}

func (r *Role) GlobalUserIds() ([]int64, error) {
	var ids []int64
	err := DB["rdb"].Table("role_global_user").Where("role_id=?", r.Id).Select("user_id").Find(&ids)
	return ids, err
}
