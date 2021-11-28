package models

import (
	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/toolkits/pkg/slice"
)

type RoleOperation struct {
	RoleName  string
	Operation string
}

func (RoleOperation) TableName() string {
	return "role_operation"
}

func RoleHasOperation(roles []string, operation string) (bool, error) {
	if len(roles) == 0 {
		return false, nil
	}

	return Exists(DB().Model(&RoleOperation{}).Where("operation = ? and role_name in ?", operation, roles))
}

func OperationsOfRole(roles []string) ([]string, error) {
	session := DB().Model(&RoleOperation{}).Select("distinct(operation) as operation")

	if !slice.ContainsString(roles, config.C.AdminRole) {
		session = session.Where("role_name in ?", roles)
	}

	var ret []string
	err := session.Pluck("operation", &ret).Error
	return ret, err
}
