package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"xorm.io/xorm"

	"github.com/toolkits/pkg/str"
)

type Team struct {
	Id          int64     `json:"id"`
	Ident       string    `json:"ident"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	Mgmt        int       `json:"mgmt"`
	Creator     int64     `json:"creator"`
	LastUpdated time.Time `json:"last_updated" xorm:"<-"`
}

func TeamIdentByIds(ids []int64) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}

	var idents []string
	err := DB["rdb"].Table("team").In("id", ids).Select("ident").Find(&idents)
	return idents, err
}

func (t *Team) UsersTotal(query string) (int64, error) {
	session := DB["rdb"].Where("id in (select user_id from team_user where team_id=?)", t.Id)
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or dispname like ?", q, q)
	}

	return session.Count(new(User))
}

type TeamMember struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	IsRoot   int    `json:"is_root"`
	TeamId   int64  `json:"team_id"`
	IsAdmin  int    `json:"is_admin"`
}

// UsersGet limit和offset没有用orm来拼接，是因为获取的list会莫名其妙变多
func (t *Team) UsersGet(query string, limit, offset int) ([]TeamMember, error) {
	if query != "" {
		sql := "select user.id, user.username, user.dispname, user.phone, user.email, user.im, user.is_root, team_user.team_id, team_user.is_admin from user, team_user where user.id=team_user.user_id and team_user.team_id=? and (user.username like ? or user.dispname like ?) order by user.username"
		q := "%" + query + "%"
		var objs []TeamMember
		err := DB["rdb"].SQL(sql, t.Id, q, q).Find(&objs)
		if err != nil {
			return nil, err
		}
		end := offset + limit
		total := len(objs)
		if end > total {
			end = total
		}
		return objs[offset:end], nil
	}

	sql := "select user.id, user.username, user.dispname, user.phone, user.email, user.im, user.is_root, team_user.team_id, team_user.is_admin from user, team_user where user.id=team_user.user_id and team_user.team_id=? order by user.username"
	var objs []TeamMember
	err := DB["rdb"].SQL(sql, t.Id).Find(&objs)
	if err != nil {
		return nil, err
	}

	end := offset + limit
	total := len(objs)
	if end > total {
		end = total
	}
	return objs[offset:end], nil
}

func (t *Team) Del() error {
	session := DB["rdb"].NewSession()
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
	if len(t.Ident) > 64 {
		return fmt.Errorf("ident too long")
	}

	if len(t.Name) > 64 {
		return fmt.Errorf("name too long")
	}

	if len(t.Note) > 255 {
		return fmt.Errorf("note too long")
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

	if str.Dangerous(t.Note) {
		return fmt.Errorf("note dangerous")
	}

	if strings.ContainsAny(t.Ident, ".%/") {
		return fmt.Errorf("ident invalid")
	}

	return nil
}

func (t *Team) Modify(name, note string, mgmt int) error {
	t.Name = name
	t.Note = note
	t.Mgmt = mgmt

	if err := t.CheckFields(); err != nil {
		return err
	}

	err := t.Update("name", "note", "mgmt")

	return err
}

func (t *Team) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", t.Id).Cols(cols...).Update(t)
	return err
}

func (t *Team) BindUser(ids []int64, isadmin int) error {
	if ids == nil {
		return nil
	}

	cnt := len(ids)
	if cnt == 0 {
		return nil
	}

	if t.Mgmt == 0 && isadmin == 1 {
		return fmt.Errorf("isadmin invalid cause mgmt=0")
	}

	ids, err := safeUserIds(ids)
	if err != nil {
		return err
	}

	session := DB["rdb"].NewSession()
	defer session.Close()
	for i := 0; i < cnt; i++ {
		if err := teamUserBind(session, t.Id, ids[i], isadmin); err != nil {
			return err
		}
	}

	return nil
}

func (t *Team) UnbindUser(ids []int64) error {
	if ids == nil || len(ids) == 0 {
		return nil
	}

	_, err := DB["rdb"].Where("team_id=?", t.Id).In("user_id", ids).Delete(new(TeamUser))
	return err
}

func TeamAdd(ident, name, note string, mgmt int, creator int64) (int64, error) {
	t := Team{
		Ident:   ident,
		Name:    name,
		Note:    note,
		Mgmt:    mgmt,
		Creator: creator,
	}

	if err := t.CheckFields(); err != nil {
		return 0, err
	}

	cnt, err := DB["rdb"].Where("ident=?", ident).Count(new(Team))
	if err != nil {
		return 0, err
	}

	if cnt > 0 {
		return 0, fmt.Errorf("%s already exists", ident)
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err = session.Begin(); err != nil {
		return 0, err
	}

	if _, err = session.Insert(&t); err != nil {
		session.Rollback()
		return 0, err
	}

	if err = teamUserBind(session, t.Id, creator, 1); err != nil {
		session.Rollback()
		return 0, err
	}

	return t.Id, session.Commit()
}

func teamUserBind(session *xorm.Session, teamid, userid int64, isadmin int) error {
	var link TeamUser
	has, err := session.Where("team_id=? and user_id=?", teamid, userid).Get(&link)
	if err != nil {
		return err
	}

	if has && isadmin != link.IsAdmin {
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

func TeamGet(where string, args ...interface{}) (*Team, error) {
	var obj Team
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
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
		return DB["rdb"].Where("ident like ? or name like ? or note like ?", q, q, q).Count(new(Team))
	}

	return DB["rdb"].Count(new(Team))
}

func TeamTotalInIds(ids []int64, query string) (int64, error) {
	session := DB["rdb"].In("id", ids)
	if query != "" {
		q := "%" + query + "%"
		return session.Where("ident like ? or name like ? or note like ?", q, q, q).Count(new(Team))
	}

	return session.Count(new(Team))
}

func TeamGets(query string, limit, offset int) ([]Team, error) {
	session := DB["rdb"].Limit(limit, offset).OrderBy("ident")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("ident like ? or name like ? or note like ?", q, q, q)
	}

	var objs []Team
	err := session.Find(&objs)
	return objs, err
}

func TeamGetsInIds(ids []int64, query string, limit, offset int) ([]Team, error) {
	session := DB["rdb"].In("id", ids).Limit(limit, offset).OrderBy("ident")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("ident like ? or name like ? or note like ?", q, q, q)
	}

	var objs []Team
	err := session.Find(&objs)
	return objs, err
}

func TeamGetByIds(ids []int64) ([]Team, error) {
	if ids == nil || len(ids) == 0 {
		return []Team{}, nil
	}

	var objs []Team
	err := DB["rdb"].In("id", ids).OrderBy("ident").Find(&objs)
	return objs, err
}

func GetTeamsNameByIds(ids string) ([]string, error) {
	idsStrArr := strings.Split(ids, ",")
	teamIds := []int64{}
	for _, tid := range idsStrArr {
		id, _ := strconv.ParseInt(tid, 10, 64)
		teamIds = append(teamIds, id)
	}

	var names []string
	teams, err := TeamGetByIds(teamIds)
	if err != nil {
		return names, err
	}
	for _, team := range teams {
		names = append(names, team.Name)
	}
	return names, err
}
