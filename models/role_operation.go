package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/slice"
)

type RoleOperation struct {
	RoleName  string
	Operation string
}

func (RoleOperation) TableName() string {
	return "role_operation"
}

func (r *RoleOperation) DB2FE() error {
	return nil
}

func RoleHasOperation(ctx *ctx.Context, roles []string, operation string) (bool, error) {
	if len(roles) == 0 {
		return false, nil
	}

	return Exists(DB(ctx).Model(&RoleOperation{}).Where("operation = ? and role_name in ?", operation, roles))
}

func OperationsOfRole(ctx *ctx.Context, roles []string) ([]string, error) {
	session := DB(ctx).Model(&RoleOperation{}).Select("distinct(operation) as operation")

	if !slice.ContainsString(roles, AdminRole) {
		session = session.Where("role_name in ?", roles)
	}

	var ret []string
	err := session.Pluck("operation", &ret).Error
	return ret, err
}

func RoleOperationBind(ctx *ctx.Context, roleName string, operation []string) error {
	tx := DB(ctx).Begin()

	if err := tx.Where("role_name = ?", roleName).Delete(&RoleOperation{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if len(operation) == 0 {
		return tx.Commit().Error
	}

	var ops []RoleOperation
	for _, op := range operation {
		ops = append(ops, RoleOperation{
			RoleName:  roleName,
			Operation: op,
		})
	}

	if err := tx.Create(&ops).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
