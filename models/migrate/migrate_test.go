package migrate

import (
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestInsertPermPoints(t *testing.T) {
	db, err := gorm.Open(mysql.Open("root:1234@tcp(127.0.0.1:3306)/n9e_v6?charset=utf8mb4&parseTime=True&loc=Local&allowNativePasswords=true"), &gorm.Config{NamingStrategy: schema.NamingStrategy{
		SingularTable: true,
	}})
	if err != nil {
		fmt.Printf("failed to connect database: %v", err)
	}

	var ops []models.RoleOperation
	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/alert-mutes/put",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/log/index-patterns",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/help/variable-configs",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Admin",
		Operation: "/permissions",
	})

	ops = append(ops, models.RoleOperation{
		RoleName:  "Standard",
		Operation: "/ibex-settings",
	})
	//db = db.Debug()
	var results []struct {
		RoleName  string
		Operation string
		Count     int
	}

	err = db.Model(&models.RoleOperation{}).
		Select("role_name, operation, COUNT(*) as count").
		Group("role_name, operation").
		Having("COUNT(*) > 0").
		Scan(&results).Error

	if err != nil {
		fmt.Printf("query failed: %v\n", err)
	}

	roleOperationMap := make(map[models.RoleOperation]bool, len(results))

	for _, result := range results {
		if result.Count > 1 {
			fmt.Printf("[role_operation count abnormal]RoleName: %s, Operation: %s, Count: %d\n", result.RoleName, result.Operation, result.Count)
		}
		roleOperationMap[models.RoleOperation{
			RoleName:  result.RoleName,
			Operation: result.Operation,
		}] = true
		fmt.Printf("RoleName: %s, Operation: %s, Count: %d\n", result.RoleName, result.Operation, result.Count)
	}

	for _, op := range ops {
		exists := false
		if _, ok := roleOperationMap[op]; ok {
			exists = true
		}

		fmt.Println("******************************************exists: ", exists)

		if exists {
			continue
		}

		if err != nil {
			fmt.Printf("insert role operation failed, %v", err)
		}
	}

	fmt.Println("use Model.Where.Count")
	for _, op := range ops {

		var count int64
		err = db.Model(&models.RoleOperation{}).
			Where("role_name = ? AND operation = ?", op.RoleName, op.Operation).
			Count(&count).Error

		if err != nil {
			fmt.Printf("query failed: %v\n", err)
			continue
		}

		exists := count > 0
		fmt.Println("******************************************exists: ", exists)

		if exists {
			continue
		}

		if err != nil {
			fmt.Printf("insert role operation failed, %v", err)
		}
	}
}
