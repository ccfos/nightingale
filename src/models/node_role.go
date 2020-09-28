package models

import "fmt"

type NodeRole struct {
	Id       int64  `json:"id"`
	NodeId   int64  `json:"node_id"`
	Username string `json:"username"`
	RoleId   int64  `json:"role_id"`
	NodePath string `xorm:"<- 'node_path'" json:"node_path"`
	RoleTxt  string `xorm:"-" json:"role_txt"`
}

func (nr *NodeRole) Save() error {
	cnt, err := DB["rdb"].Where("node_id=? and username=? and role_id=?", nr.NodeId, nr.Username, nr.RoleId).Count(new(NodeRole))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("user already has this role")
	}

	_, err = DB["rdb"].Insert(nr)
	return err
}

func NodeRoleExists(nodeIds, roleIds []int64, username string) (bool, error) {
	num, err := DB["rdb"].In("node_id", nodeIds).In("role_id", roleIds).Where("username=?", username).Count(new(NodeRole))
	return num > 0, err
}

func NodeRoleDel(nodeId, roleId int64, username string) error {
	_, err := DB["rdb"].Where("node_id=? and role_id=? and username=?", nodeId, roleId, username).Delete(new(NodeRole))
	return err
}

// NodeIdsBindingUsername 某人在哪些节点配置过权限
func NodeIdsBindingUsername(username string) ([]int64, error) {
	var ids []int64
	err := DB["rdb"].Table("node_role").Where("username=?", username).Select("node_id").Find(&ids)
	return ids, err
}

// NodeIdsBindingUsernameWithRoles 我以某些角色的名义绑定在哪些节点
func NodeIdsBindingUsernameWithRoles(username string, roleIds []int64) ([]int64, error) {
	if len(roleIds) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["rdb"].Table("node_role").Where("username=?", username).In("role_id", roleIds).Select("node_id").Find(&ids)
	return ids, err
}

// NodeIdsBindingUsernameWithOp 我在哪些节点上有这个操作权限
func NodeIdsBindingUsernameWithOp(username, op string) ([]int64, error) {
	roleIds, err := RoleIdsHasOp(op)
	if err != nil {
		return []int64{}, err
	}

	return NodeIdsBindingUsernameWithRoles(username, roleIds)
}
