package models

import (
	"github.com/toolkits/pkg/logger"
	"xorm.io/builder"
)

type RoleOperation struct {
	RoleName  string
	Operation string
}

func (RoleOperation) TableName() string {
	return "role_operation"
}

func RoleHasOperation(roles []string, operation string) (bool, error) {
	cond := builder.NewCond()
	cond = cond.And(builder.In("role_name", roles))
	cond = cond.And(builder.Eq{"operation": operation})
	num, err := DB.Where(cond).Count(new(RoleOperation))
	if err != nil {
		logger.Errorf("mysql.error query role_operation fail: %v", err)
		return false, internalServerError
	}
	return num > 0, nil
}
