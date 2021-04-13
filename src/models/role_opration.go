package models

import (
	"github.com/toolkits/pkg/slice"
)

type RoleOperation struct {
	Id        int64  `json:"id"`
	RoleId    int64  `json:"role_id"`
	Operation string `json:"operation"`
}

func RoleOperationAll() ([]RoleOperation, error) {
	var objs []RoleOperation
	err := DB["rdb"].OrderBy("id").Find(&objs)
	return objs, err
}

func OperationsOfRoles(rids []int64) ([]string, error) {
	if len(rids) == 0 {
		return []string{}, nil
	}

	var ops []string
	err := DB["rdb"].Table("role_operation").In("role_id", rids).Select("operation").Find(&ops)
	return ops, err
}

// RoleIdsHasOp 看哪些role里边包含operation
func RoleIdsHasOp(op string) ([]int64, error) {
	var ids []int64
	err := DB["rdb"].Table("role_operation").Where("operation=?", op).Select("role_id").Find(&ids)
	return ids, err
}

func RoleBiggerThan(roleid int64) ([]int64, error) {
	var objs []RoleOperation
	err := DB["rdb"].Find(&objs)
	if err != nil {
		return nil, err
	}

	m := make(map[int64][]string)
	count := len(objs)
	for i := 0; i < count; i++ {
		m[objs[i].RoleId] = append(m[objs[i].RoleId], objs[i].Operation)
	}

	ret := []int64{}
	ops := m[roleid]

	for rid := range m {
		if slice.ContainsSlice(ops, m[rid]) {
			ret = append(ret, rid)
		}
	}

	return ret, nil
}
