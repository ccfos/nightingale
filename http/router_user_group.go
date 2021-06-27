package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func userGroupListGet(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	query := queryStr(c, "query", "")

	total, err := models.UserGroupTotal(query)
	dangerous(err)

	list, err := models.UserGroupGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

// 与我相关的用户组，我创建的，或者我是其中一员
// 这个量不大，搜索和分页都放在前端来做，后端搞起来比较麻烦
func userGroupMineGet(c *gin.Context) {
	list, err := loginUser(c).MyUserGroups()
	renderData(c, list, err)
}

type userGroupForm struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

func userGroupAdd(c *gin.Context) {
	var f userGroupForm
	bind(c, &f)

	me := loginUser(c)

	ug := models.UserGroup{
		Name:     f.Name,
		Note:     f.Note,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	dangerous(ug.Add())

	// 顺便把创建者也作为团队的一员，失败了也没关系，用户会重新添加成员
	models.UserGroupMemberAdd(ug.Id, me.Id)

	renderMessage(c, nil)
}

func userGroupPut(c *gin.Context) {
	var f userGroupForm
	bind(c, &f)

	me := loginUser(c)
	ug := UserGroup(urlParamInt64(c, "id"))

	can, err := me.CanModifyUserGroup(ug)
	dangerous(err)

	if !can {
		bomb(http.StatusForbidden, "forbidden")
	}

	if ug.Name != f.Name {
		// 如果name发生变化，需要检查这个新name是否与别的group重名
		num, err := models.UserGroupCount("name=? and id<>?", f.Name, ug.Id)
		dangerous(err)

		if num > 0 {
			bomb(200, "UserGroup %s already exists", f.Name)
		}
	}

	ug.Name = f.Name
	ug.Note = f.Note
	ug.UpdateBy = me.Username
	ug.UpdateAt = time.Now().Unix()

	renderMessage(c, ug.Update("name", "note", "update_at", "update_by"))
}

// 不但返回UserGroup的信息，也把成员信息返回，成员不会特别多，所以，
// 成员全部返回，由前端分页、查询
func userGroupGet(c *gin.Context) {
	ug := UserGroup(urlParamInt64(c, "id"))

	ids, err := ug.MemberIds()
	dangerous(err)

	users, err := models.UserGetsByIds(ids)

	renderData(c, gin.H{
		"users":      users,
		"user_group": ug,
	}, err)
}

func userGroupMemberAdd(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()

	me := loginUser(c)
	ug := UserGroup(urlParamInt64(c, "id"))

	can, err := me.CanModifyUserGroup(ug)
	dangerous(err)

	if !can {
		bomb(http.StatusForbidden, "forbidden")
	}

	dangerous(ug.AddMembers(f.Ids))

	// 用户组的成员发生变化，相当于更新了用户组
	// 如果更新失败了直接忽略，不是啥大事
	ug.UpdateAt = time.Now().Unix()
	ug.UpdateBy = me.Username
	ug.Update("update_at", "update_by")

	renderMessage(c, nil)
}

func userGroupMemberDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()

	me := loginUser(c)
	ug := UserGroup(urlParamInt64(c, "id"))

	can, err := me.CanModifyUserGroup(ug)
	dangerous(err)

	if !can {
		bomb(http.StatusForbidden, "forbidden")
	}

	dangerous(ug.DelMembers(f.Ids))

	// 用户组的成员发生变化，相当于更新了用户组
	// 如果更新失败了直接忽略，不是啥大事
	ug.UpdateAt = time.Now().Unix()
	ug.UpdateBy = me.Username
	ug.Update("update_at", "update_by")

	renderMessage(c, nil)
}

func userGroupDel(c *gin.Context) {
	me := loginUser(c)
	ug := UserGroup(urlParamInt64(c, "id"))

	can, err := me.CanModifyUserGroup(ug)
	dangerous(err)

	if !can {
		bomb(http.StatusForbidden, "forbidden")
	}

	renderMessage(c, ug.Del())
}
