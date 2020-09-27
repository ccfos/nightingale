package http

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
)

func teamAllGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	total, err := models.TeamTotal(query)
	dangerous(err)

	list, err := models.TeamGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func teamMineGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	user := loginUser(c)

	teamIds, err := models.TeamIdsByUserId(user.Id)
	dangerous(err)

	if len(teamIds) == 0 {
		renderZeroPage(c)
		return
	}

	total, err := models.TeamTotalInIds(teamIds, query)
	dangerous(err)

	list, err := models.TeamGetsInIds(teamIds, query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func teamDetail(c *gin.Context) {
	query := queryStr(c, "query", "")
	limit := queryInt(c, "limit", 20)

	team := Team(urlParamInt64(c, "id"))

	total, err := team.UsersTotal(query)
	dangerous(err)

	list, err := team.UsersGet(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
		"team":  team,
	}, nil)
}

type teamForm struct {
	Ident string `json:"ident"`
	Name  string `json:"name"`
	Note  string `json:"note"`
	Mgmt  int    `json:"mgmt"`
}

func teamAddPost(c *gin.Context) {
	var f teamForm
	bind(c, &f)

	me := loginUser(c)

	lastid, err := models.TeamAdd(f.Ident, f.Name, f.Note, f.Mgmt, me.Id)
	if err == nil {
		go models.OperationLogNew(me.Username, "team", lastid, fmt.Sprintf("TeamCreate ident: %s name: %s", f.Ident, f.Name))
	}

	renderMessage(c, err)
}

func teamPut(c *gin.Context) {
	me := loginUser(c)

	var f teamForm
	bind(c, &f)

	t := Team(urlParamInt64(c, "id"))

	can, err := me.CanModifyTeam(t)
	dangerous(err)

	if !can {
		bomb("no privilege")
	}

	arr := make([]string, 0, 2)
	if f.Name != t.Name {
		arr = append(arr, fmt.Sprintf("name: %s -> %s", t.Name, f.Name))
	}

	if f.Note != t.Note {
		arr = append(arr, fmt.Sprintf("note: %s -> %s", t.Note, f.Note))
	}

	err = t.Modify(f.Name, f.Note, f.Mgmt)
	if err == nil && len(arr) > 0 {
		content := strings.Join(arr, ", ")
		go models.OperationLogNew(me.Username, "team", t.Id, "TeamModify "+content)
	}

	renderMessage(c, err)
}

type teamUserBindForm struct {
	AdminIds  []int64 `json:"admin_ids"`
	MemberIds []int64 `json:"member_ids"`
}

func teamUserBind(c *gin.Context) {
	me := loginUser(c)

	var f teamUserBindForm
	bind(c, &f)

	team := Team(urlParamInt64(c, "id"))

	can, err := me.CanModifyTeam(team)
	dangerous(err)

	if !can {
		bomb("no privilege")
	}

	if f.AdminIds != nil && len(f.AdminIds) > 0 {
		dangerous(team.BindUser(f.AdminIds, 1))
	}

	if f.MemberIds != nil && len(f.MemberIds) > 0 {
		dangerous(team.BindUser(f.MemberIds, 0))
	}

	renderMessage(c, nil)
}

type teamUserUnbindForm struct {
	UserIds []int64 `json:"user_ids"`
}

func teamUserUnbind(c *gin.Context) {
	me := loginUser(c)

	var f teamUserUnbindForm
	bind(c, &f)

	team := Team(urlParamInt64(c, "id"))

	can, err := me.CanModifyTeam(team)
	dangerous(err)

	if !can {
		bomb("no privilege")
	}

	renderMessage(c, team.UnbindUser(f.UserIds))
}

func teamDel(c *gin.Context) {
	me := loginUser(c)

	t, err := models.TeamGet("id=?", urlParamInt64(c, "id"))
	dangerous(err)

	if t == nil {
		renderMessage(c, nil)
		return
	}

	can, err := me.CanModifyTeam(t)
	dangerous(err)

	if !can {
		bomb("no privilege")
	}

	err = t.Del()
	if err == nil {
		go models.OperationLogNew(me.Username, "team", t.Id, fmt.Sprintf("TeamDelete ident: %s name: %s", t.Ident, t.Name))
	}

	renderMessage(c, err)
}

func belongTeamsGet(c *gin.Context) {
	username := queryStr(c, "username")
	isadminStr := queryStr(c, "is_admin", "")

	user, err := models.UserGet("username=?", username)
	dangerous(err)

	if user == nil {
		bomb("no such username[%s]", username)
	}

	var ids []int64
	if isadminStr == "" {
		ids, err = models.TeamIdsByUserId(user.Id)
		dangerous(err)
	} else {
		isadminInt, err := strconv.Atoi(isadminStr)
		dangerous(err)

		ids, err = models.TeamIdsByUserId(user.Id, isadminInt)
		dangerous(err)
	}

	ret, err := models.TeamGetByIds(ids)
	renderData(c, ret, err)
}

func teamGetByIdent(c *gin.Context) {
	ident := urlParamStr(c, "ident")
	team, err := models.TeamGet("ident=?", ident)
	renderData(c, team, err)
}

func isTeamMember(c *gin.Context) {
	username := queryStr(c, "username")
	teamIdent := queryStr(c, "team")

	user, err := models.UserGet("username=?", username)
	dangerous(err)

	if user == nil {
		bomb("no such username[%s]", username)
	}

	team, err := models.TeamGet("ident=?", teamIdent)
	dangerous(err)

	if team == nil {
		bomb("no such team[%s]", teamIdent)
	}

	has, err := models.TeamHasMember(team.Id, user.Id)
	renderData(c, has, err)
}

func v1TeamGetByIds(c *gin.Context) {
	ids := queryStr(c, "ids")
	teams, err := models.TeamGetByIds(str.IdsInt64(ids))
	renderData(c, teams, err)
}
