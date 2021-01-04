package http

import (
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

func rolesUnderNodeGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	username := queryStr(c, "username", "")
	node := Node(urlParamInt64(c, "id"))

	total, err := node.RoleTotal(username)
	dangerous(err)

	list, err := node.RoleList(username, limit, offset(c, limit))
	dangerous(err)

	m, err := models.RoleMap("local")
	dangerous(err)

	size := len(list)
	var usernames []string
	for i := 0; i < size; i++ {
		usernames = append(usernames, list[i].Username)
	}

	users, err := models.UserGetByNames(usernames)
	dangerous(err)

	usersMap := make(map[string]models.User)
	for i := 0; i < len(users); i++ {
		usersMap[users[i].Username] = users[i]
	}

	for i := 0; i < size; i++ {
		list[i].RoleTxt = m[list[i].RoleId]
		if user, exists := usersMap[list[i].Username]; exists {
			list[i].Dispname = user.Dispname
		}
	}

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type roleUnderNodeForm struct {
	Usernames []string `json:"usernames"`
	RoleId    int64    `json:"role_id"`
}

func rolesUnderNodePost(c *gin.Context) {
	var f roleUnderNodeForm
	bind(c, &f)

	node := Node(urlParamInt64(c, "id"))
	role := Role(f.RoleId)

	me := loginUser(c)
	me.CheckPermByNode(node, "rdb_perm_grant")

	count := len(f.Usernames)
	for i := 0; i < count; i++ {
		user, err := models.UserGet("username=?", f.Usernames[i])
		dangerous(err)

		if user == nil {
			bomb("no such user: %s", f.Usernames[i])
		}

		nodeRole := &models.NodeRole{
			NodeId:   node.Id,
			Username: f.Usernames[i],
			RoleId:   f.RoleId,
		}
		err = nodeRole.Save()
		if err == nil {
			detail := fmt.Sprintf("NodeRoleBind node: %s, username: %s, role: %s", node.Path, f.Usernames[i], role.Name)
			go models.OperationLogNew(me.Username, "node", node.Id, detail)
		}
		dangerous(err)
	}

	renderMessage(c, nil)
}

type roleUnderNodeDelForm struct {
	Username string `json:"username"`
	RoleId   int64  `json:"role_id"`
}

func rolesUnderNodeDel(c *gin.Context) {
	var f roleUnderNodeDelForm
	bind(c, &f)

	node := Node(urlParamInt64(c, "id"))
	role := Role(f.RoleId)

	me := loginUser(c)
	if me.Username != f.Username {
		// 即使我没有rdb_perm_grant权限，我也可以删除我自己的权限，所以，两个username不同的时候才需要鉴权
		me.CheckPermByNode(node, "rdb_perm_grant")
	}

	err := models.NodeRoleDel(node.Id, f.RoleId, f.Username)
	if err == nil {
		detail := fmt.Sprintf("NodeRoleUnbind node: %s, username: %s, role: %s", node.Path, f.Username, role.Name)
		go models.OperationLogNew(me.Username, "node", node.Id, detail)
	}

	renderMessage(c, err)
}
