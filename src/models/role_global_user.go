package models

type RoleGlobalUser struct {
	RoleId int64 `json:"role_id" xorm:"'role_id'"`
	UserId int64 `json:"user_id" xorm:"'user_id'"`
}

func RoleGlobalUserAll() ([]RoleGlobalUser, error) {
	var objs []RoleGlobalUser
	err := DB["rdb"].Find(&objs)
	return objs, err
}

// UserHasGlobalRole 查看某个用户是否有某个全局角色
func UserHasGlobalRole(userId int64, roleIds []int64) (bool, error) {
	cnt, err := DB["rdb"].Where("user_id=?", userId).In("role_id", roleIds).Count(new(RoleGlobalUser))
	return cnt > 0, err
}

func RoleIdsGetByUserId(userId int64) ([]int64, error) {
	var roleIds []int64
	err := DB["rdb"].Table(new(RoleGlobalUser)).Where("user_id=?", userId).Select("role_id").Find(&roleIds)
	return roleIds, err
}

func UserIdsGetByRoleIds(roleIds []int64) ([]int64, error) {
	var ids []int64
	err := DB["rdb"].Table(new(RoleGlobalUser)).In("role_id", roleIds).Select("user_id").Find(&ids)
	return ids, err
}
