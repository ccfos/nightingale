package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func checkBusiGroupPerm(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	bg := BusiGroup(ginx.UrlParamInt64(c, "id"))

	can, err := me.CanDoBusiGroup(bg, ginx.UrlParamStr(c, "perm"))
	ginx.NewRender(c).Data(can, err)
}

// Return all, front-end search and paging
// I'm creator or member
func userGroupGets(c *gin.Context) {
	limit := ginx.QueryInt(c, "limit", defaultLimit)
	query := ginx.QueryStr(c, "query", "")

	me := c.MustGet("user").(*models.User)
	lst, err := me.UserGroups(limit, query)

	ginx.NewRender(c).Data(lst, err)
}

type userGroupForm struct {
	Name string `json:"name" binding:"required"`
	Note string `json:"note"`
}

func userGroupAdd(c *gin.Context) {
	var f userGroupForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)

	ug := models.UserGroup{
		Name:     f.Name,
		Note:     f.Note,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	err := ug.Add()
	if err == nil {
		// Even failure is not a big deal
		models.UserGroupMemberAdd(ug.Id, me.Id)
	}

	ginx.NewRender(c).Data(ug.Id, err)
}

func userGroupPut(c *gin.Context) {
	var f userGroupForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	ug := c.MustGet("user_group").(*models.UserGroup)

	if ug.Name != f.Name {
		// name changed, check duplication
		num, err := models.UserGroupCount("name=? and id<>?", f.Name, ug.Id)
		ginx.Dangerous(err)

		if num > 0 {
			ginx.Bomb(http.StatusOK, "UserGroup already exists")
		}
	}

	ug.Name = f.Name
	ug.Note = f.Note
	ug.UpdateBy = me.Username
	ug.UpdateAt = time.Now().Unix()

	ginx.NewRender(c).Message(ug.Update("Name", "Note", "UpdateAt", "UpdateBy"))
}

// Return all members, front-end search and paging
func userGroupGet(c *gin.Context) {
	ug := UserGroup(ginx.UrlParamInt64(c, "id"))

	ids, err := models.MemberIds(ug.Id)
	ginx.Dangerous(err)

	users, err := models.UserGetsByIds(ids)

	ginx.NewRender(c).Data(gin.H{
		"users":      users,
		"user_group": ug,
	}, err)
}

func userGroupDel(c *gin.Context) {
	ug := c.MustGet("user_group").(*models.UserGroup)
	ginx.NewRender(c).Message(ug.Del())
}

func userGroupMemberAdd(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	me := c.MustGet("user").(*models.User)
	ug := c.MustGet("user_group").(*models.UserGroup)

	err := ug.AddMembers(f.Ids)
	if err == nil {
		ug.UpdateAt = time.Now().Unix()
		ug.UpdateBy = me.Username
		ug.Update("UpdateAt", "UpdateBy")
	}

	ginx.NewRender(c).Message(err)
}

func userGroupMemberDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	me := c.MustGet("user").(*models.User)
	ug := c.MustGet("user_group").(*models.UserGroup)

	err := ug.DelMembers(f.Ids)
	if err == nil {
		ug.UpdateAt = time.Now().Unix()
		ug.UpdateBy = me.Username
		ug.Update("UpdateAt", "UpdateBy")
	}

	ginx.NewRender(c).Message(err)
}
