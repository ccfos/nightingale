package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

// 全局角色列表，谁都可以查看，没有权限限制
func globalRoleGet(c *gin.Context) {
	list, err := models.RoleFind("global")
	renderData(c, list, err)
}

// 局部角色列表，谁都可以查看，没有权限限制
func localRoleGet(c *gin.Context) {
	list, err := models.RoleFind("local")
	renderData(c, list, err)
}

type roleForm struct {
	Name       string   `json:"name"`
	Note       string   `json:"note"`
	Cate       string   `json:"cate"`
	Operations []string `json:"operations"`
}

// 创建某个角色，只有超管有权限
func roleAddPost(c *gin.Context) {
	var f roleForm
	bind(c, &f)

	r := models.Role{
		Name: f.Name,
		Note: f.Note,
		Cate: f.Cate,
	}

	dangerous(r.Save(f.Operations))

	renderData(c, gin.H{"id": r.Id}, nil)
}

// 修改某个角色，主要是改动一些基本信息，只有超管有权限
func rolePut(c *gin.Context) {
	var f roleForm
	bind(c, &f)

	obj := Role(urlParamInt64(c, "id"))

	renderMessage(c, obj.Modify(f.Name, f.Note, f.Cate, f.Operations))
}

// 删除某个角色，会删除很多关联表
func roleDel(c *gin.Context) {
	obj, err := models.RoleGet("id=?", urlParamInt64(c, "id"))
	dangerous(err)

	if obj == nil {
		renderMessage(c, nil)
		return
	}

	renderMessage(c, obj.Del())
}

// 点击某个角色查看详情页面
func roleDetail(c *gin.Context) {
	obj := Role(urlParamInt64(c, "id"))

	ops, err := models.OperationsOfRoles([]int64{obj.Id})
	dangerous(err)

	renderData(c, gin.H{
		"role":       obj,
		"operations": ops,
	}, nil)
}

// 全局角色下面的用户列表，只有管理员可以查看的一个页面
func roleGlobalUsersGet(c *gin.Context) {
	rid := urlParamInt64(c, "id")
	role := Role(rid)

	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	ids, err := role.GlobalUserIds()
	dangerous(err)

	if len(ids) == 0 {
		renderData(c, gin.H{
			"list":  []models.User{},
			"total": 0,
			"role":  role,
		}, nil)
		return
	}

	total, err := models.UserSearchTotalInIds(ids, query)
	dangerous(err)

	list, err := models.UserSearchListInIds(ids, query, limit, offset(c, limit))
	dangerous(err)

	for i := 0; i < len(list); i++ {
		list[i].UUID = ""
	}

	renderData(c, gin.H{
		"list":  list,
		"total": total,
		"role":  role,
	}, nil)
}

// 全局角色绑定一些人，把某些人设置为某个全局角色
func roleGlobalUsersBind(c *gin.Context) {
	var f idsForm
	bind(c, &f)

	obj, err := models.RoleGet("id=?", urlParamInt64(c, "id"))
	dangerous(err)

	if obj == nil {
		bomb("no such role")
	}

	renderMessage(c, obj.BindUsers(f.Ids))
}

// 把某些人从某个全局角色踢掉
func roleGlobalUsersUnbind(c *gin.Context) {
	var f idsForm
	bind(c, &f)

	obj, err := models.RoleGet("id=?", urlParamInt64(c, "id"))
	dangerous(err)

	if obj == nil {
		bomb("no such role")
	}

	renderMessage(c, obj.UnbindUsers(f.Ids))
}
