package model

import (
	"fmt"

	"xorm.io/xorm"

	jsoniter "github.com/json-iterator/go"
	"github.com/toolkits/pkg/str"
)

type Team struct {
	Id         int64  `json:"id"`
	Ident      string `json:"ident"`
	Name       string `json:"name"`
	Mgmt       int    `json:"mgmt"`
	AdminObjs  []User `json:"admin_objs" xorm:"-"`
	MemberObjs []User `json:"member_objs" xorm:"-"`
}

func (t *Team) Del() error {
	session := DB["uic"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM team_user WHERE team_id=?", t.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM team WHERE id=?", t.Id); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (t *Team) CheckFields() error {
	if len(t.Ident) > 255 {
		return fmt.Errorf("ident too long")
	}

	if len(t.Name) > 255 {
		return fmt.Errorf("name too long")
	}

	if t.Mgmt != 0 && t.Mgmt != 1 {
		return fmt.Errorf("mgmt invalid")
	}

	if str.Dangerous(t.Ident) {
		return fmt.Errorf("ident dangerous")
	}

	if str.Dangerous(t.Name) {
		return fmt.Errorf("name dangerous")
	}

	if !str.IsMatch(t.Ident, `^[a-z0-9\-]+$`) {
		return fmt.Errorf("ident permissible characters: [a-z0-9] and -")
	}

	return nil
}

func (t *Team) FillObjs() error {
	var tus []TeamUser
	err := DB["uic"].Where("team_id=?", t.Id).Find(&tus)
	if err != nil {
		return err
	}

	cnt := len(tus)
	for i := 0; i < cnt; i++ {
		user, err := UserGet("id", tus[i].UserId)
		if err != nil {
			return err
		}

		if user == nil {
			continue
		}

		if tus[i].IsAdmin == 1 {
			t.AdminObjs = append(t.AdminObjs, *user)
		} else {
			t.MemberObjs = append(t.MemberObjs, *user)
		}
	}

	return nil
}

func safeUserIds(ids []int64) ([]int64, error) {
	cnt := len(ids)
	ret := make([]int64, 0, cnt)
	for i := 0; i < cnt; i++ {
		user, err := UserGet("id", ids[i])
		if err != nil {
			return nil, err
		}

		if user != nil {
			ret = append(ret, ids[i])
		}
	}
	return ret, nil
}

func (t *Team) Modify(ident, name string, mgmt int, admins, members []int64) error {
	adminIds, err := safeUserIds(admins)
	if err != nil {
		return err
	}

	memberIds, err := safeUserIds(members)
	if err != nil {
		return err
	}

	if len(adminIds) == 0 && len(memberIds) == 0 {
		return fmt.Errorf("no invalid memeber ids")
	}

	if mgmt == 1 && len(adminIds) == 0 {
		return fmt.Errorf("arg[admins] is necessary")
	}

	// 如果ident有变化，就要检查是否有重名
	if ident != t.Ident {
		cnt, err := DB["uic"].Where("ident = ? and id <> ?", ident, t.Id).Count(new(Team))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return fmt.Errorf("ident[%s] already exists", ident)
		}
	}

	t.Ident = ident
	t.Name = name
	t.Mgmt = mgmt

	if err = t.CheckFields(); err != nil {
		return err
	}

	session := DB["uic"].NewSession()
	defer session.Close()

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Where("id=?", t.Id).Cols("ident", "name", "mgmt").Update(t); err != nil {
		session.Rollback()
		return err
	}

	if _, err = session.Exec("DELETE FROM team_user WHERE team_id=?", t.Id); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(adminIds); i++ {
		if err = teamUserBind(session, t.Id, adminIds[i], 1); err != nil {
			session.Rollback()
			return err
		}
	}

	for i := 0; i < len(memberIds); i++ {
		if err = teamUserBind(session, t.Id, memberIds[i], 0); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func TeamAdd(ident, name string, mgmt int, admins, members []int64) error {
	adminIds, err := safeUserIds(admins)
	if err != nil {
		return err
	}

	memberIds, err := safeUserIds(members)
	if err != nil {
		return err
	}

	if len(adminIds) == 0 && len(memberIds) == 0 {
		return fmt.Errorf("no invalid memeber ids")
	}

	if mgmt == 1 && len(adminIds) == 0 {
		return fmt.Errorf("arg[admins] is necessary")
	}

	t := Team{
		Ident: ident,
		Name:  name,
		Mgmt:  mgmt,
	}

	if err = t.CheckFields(); err != nil {
		return err
	}

	session := DB["uic"].NewSession()
	defer session.Close()

	cnt, err := session.Where("ident=?", ident).Count(new(Team))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("%s already exists", ident)
	}

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Insert(&t); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(adminIds); i++ {
		if err := teamUserBind(session, t.Id, adminIds[i], 1); err != nil {
			session.Rollback()
			return err
		}
	}

	for i := 0; i < len(memberIds); i++ {
		if err := teamUserBind(session, t.Id, memberIds[i], 0); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func teamUserBind(session *xorm.Session, teamid, userid int64, isadmin int) error {
	var tu TeamUser
	has, err := session.Where("team_id=? and user_id=?", teamid, userid).Get(&tu)
	if err != nil {
		return err
	}

	if has && isadmin != tu.IsAdmin {
		_, err = session.Exec("UPDATE team_user SET is_admin=? WHERE team_id=? and user_id=?", isadmin, teamid, userid)
		if err != nil {
			return err
		}
	}

	if !has {
		_, err = session.Insert(&TeamUser{
			TeamId:  teamid,
			UserId:  userid,
			IsAdmin: isadmin,
		})
		return err
	}

	return nil
}

func TeamGet(col string, val interface{}) (*Team, error) {
	var obj Team
	has, err := DB["uic"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func TeamTotal(query string) (int64, error) {
	if query != "" {
		q := "%" + query + "%"
		return DB["uic"].Where("ident like ? or name like ?", q, q).Count(new(Team))
	}

	return DB["uic"].Count(new(Team))
}

func TeamGets(query string, limit, offset int) ([]Team, error) {
	session := DB["uic"].Limit(limit, offset).OrderBy("ident")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("ident like ? or name like ?", q, q)
	}

	var objs []Team
	err := session.Find(&objs)
	return objs, err
}

func TeamNameGetsByIds(ids string) ([]string, error) {
	var objs []Team
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var groupIds []int64

	if err := json.Unmarshal([]byte(ids), &groupIds); err != nil {
		return nil, err
	}

	err := DB["uic"].In("id", groupIds).Cols("name").Find(&objs)
	if err != nil {
		return nil, err
	}

	names := []string{}
	for i := 0; i < len(objs); i++ {
		names = append(names, objs[i].Name)
	}
	return names, nil
}
