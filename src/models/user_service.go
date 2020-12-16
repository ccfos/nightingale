package models

import (
	"fmt"
	"strings"

	"github.com/toolkits/pkg/slice"
)

// TenantsGetByUserId 使用方：工单
func TenantsGetByUserId(id int64, withAdmins ...bool) (tenants []Node, err error) {
	// 校验用户ID是否合法
	user, err := UserGet("id=?", id)
	if err != nil {
		return tenants, err
	}

	if user == nil {
		return tenants, fmt.Errorf("no such user, id: %d", id)
	}

	// 我是管理员的节点ID
	ids1, err := NodeIdsIamAdmin(user.Id)
	if err != nil {
		return tenants, err
	}

	// 我有授权关系的节点ID
	ids2, err := NodeIdsBindingUsername(user.Username)
	if err != nil {
		return tenants, err
	}

	// 把节点ID合并之后，对合集求node列表
	nodes, err := NodeByIds(slice.MergeInt64(ids1, ids2))
	if err != nil {
		return tenants, err
	}

	// 我没有关联任何节点，表示我不属于任何租户
	count := len(nodes)
	if count == 0 {
		return tenants, nil
	}

	idents := make(map[string]struct{})
	for i := 0; i < count; i++ {
		idents[strings.Split(nodes[i].Path, ".")[0]] = struct{}{}
	}

	for ident := range idents {
		node, err := NodeGet("path=?", ident)
		if err != nil {
			return tenants, err
		}

		if len(withAdmins) > 0 && withAdmins[0] {
			err = node.FillAdmins()
			if err != nil {
				return tenants, err
			}
		}

		tenants = append(tenants, *node)
	}

	return tenants, nil
}

// UsersGetByGlobalRoleIds 使用方：工单
func UsersGetByGlobalRoleIds(ids []int64) (users []User, err error) {
	if len(ids) == 0 {
		return users, nil
	}

	userIds, err := UserIdsGetByRoleIds(ids)
	if err != nil {
		return users, err
	}

	return UserGetByIds(userIds)
}

// GlobalRolesGetByUserId 使用方：工单
func GlobalRolesGetByUserId(id int64) (roles []Role, err error) {
	roleIds, err := RoleIdsGetByUserId(id)
	if err != nil {
		return roles, err
	}

	return RoleGetByIds(roleIds)
}

// GetUsernameByToken 使用方：rbac-proxy
func GetUsernameByToken(token string) (string, error) {
	ut, err := UserTokenGet("token=?", token)
	if err != nil {
		return "", err
	}

	if ut == nil {
		return "", fmt.Errorf("no such token")
	}

	return ut.Username, nil
}

// UsernameCandoGlobalOp 使用方：RDB、rbac-proxy
func UsernameCandoGlobalOp(username, operation string) (bool, error) {
	user, err := UserGet("username=?", username)
	if err != nil {
		return false, err
	}

	if user == nil {
		return false, fmt.Errorf("no such user: %s", username)
	}

	return user.HasPermGlobal(operation)
}

func UsernameCandoNodeOp(username, operation string, nodeId int64) (bool, error) {
	user, err := UserGet("username=?", username)
	if err != nil {
		return false, err
	}

	if user == nil {
		return false, fmt.Errorf("no such user: %s", username)
	}

	node, err := NodeGet("id=?", nodeId)
	if err != nil {
		return false, err
	}

	if node == nil {
		return false, fmt.Errorf("no such node, id: %d", nodeId)
	}

	return user.HasPermByNode(node, operation)
}

func UserAndTotalGets(query, org string, limit, offset int, ids []int64) ([]User, int64, error) {
	where := "1 = 1"
	param := []interface{}{}

	if query != "" {
		q := "%" + query + "%"
		where += " and (username like ? or dispname like ? or phone like ? or email like ?)"
		param = append(param, q, q, q, q)
	}

	if org != "" {
		q := "%" + org + "%"
		where += " and organization like ?"
		param = append(param, q)

	}

	total, err := UserTotal(ids, where, param...)
	if err != nil {
		return []User{}, total, err
	}

	list, err := UserGets(ids, limit, offset, where, param...)

	return list, total, err
}
