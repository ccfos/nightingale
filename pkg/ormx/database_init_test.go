package ormx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDataBaseInit(t *testing.T) {
	tests := []struct {
		name   string
		config DBConfig
	}{
		{
			name: "MySQL",
			config: DBConfig{
				DBType: "mysql",
				DSN:    "root:1234@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local&allowNativePasswords=true",
			},
		},
		{
			name: "Postgres",
			config: DBConfig{
				DBType: "postgres",
				DSN:    "host=127.0.0.1 port=5432 user=postgres dbname=test password=1234 sslmode=disable",
			},
		},
		{
			name: "SQLite",
			config: DBConfig{
				DBType: "sqlite",
				DSN:    "./test.db",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createDatabase(tt.config, &gorm.Config{})
			assert.NoError(t, err)
			var dialector gorm.Dialector
			switch tt.config.DBType {
			case "mysql":
				dialector = mysql.Open(tt.config.DSN)
			case "postgres":
				dialector = postgres.Open(tt.config.DSN)
			case "sqlite":
				dialector = sqlite.Open(tt.config.DSN)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			assert.NoError(t, err)
			DataBaseInit(tt.config, db)
		})
	}
}
