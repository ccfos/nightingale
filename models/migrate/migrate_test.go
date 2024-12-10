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

	db = db.Debug()
	for _, op := range ops {
		var count int64

		err := db.Raw("SELECT COUNT(*) FROM role_operation WHERE operation = ? AND role_name = ?",
			op.Operation, op.RoleName).Scan(&count).Error
		fmt.Printf("count: %d\n", count)

		if err != nil {
			fmt.Printf("check role operation exists failed, %v", err)
			continue
		}

		if count > 0 {
			continue
		}

		err = db.Create(&op).Error
		if err != nil {
			fmt.Printf("insert role operation failed, %v", err)
		}
	}
}
