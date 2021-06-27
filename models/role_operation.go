package models

import "github.com/toolkits/pkg/logger"

type RoleOperation struct {
	RoleName  string
	Operation string
}

func (RoleOperation) TableName() string {
	return "role_operation"
}

func RoleHasOperation(roleName, operation string) (bool, error) {
	num, err := DB.Where("role_name=? and operation=?", roleName, operation).Count(new(RoleOperation))
	if err != nil {
		logger.Errorf("mysql.error query role_operation fail: %v", err)
		return false, internalServerError
	}
	return num > 0, nil
}
