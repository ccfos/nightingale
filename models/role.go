package models

import "github.com/toolkits/pkg/logger"

type Role struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
}

func (Role) TableName() string {
	return "role"
}

func RoleGets(where string, args ...interface{}) ([]Role, error) {
	var objs []Role
	err := DB.Where(where, args...).OrderBy("name").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: list role fail: %v", err)
		return objs, internalServerError
	}
	return objs, nil
}

func RoleGetsAll() ([]Role, error) {
	return RoleGets("")
}
