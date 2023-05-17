package ctx

import (
	"context"

	"gorm.io/gorm"
)

type Context struct {
	DB       *gorm.DB
	Addrs    []string
	Ctx      context.Context
	IsCenter bool
}

func NewContext(ctx context.Context, db *gorm.DB, isCenter bool, addrs ...string) *Context {
	return &Context{
		Ctx:      ctx,
		DB:       db,
		Addrs:    addrs,
		IsCenter: isCenter,
	}
}

// set db to Context
func (c *Context) SetDB(db *gorm.DB) {
	c.DB = db
}

// get context from Context
func (c *Context) GetContext() context.Context {
	return c.Ctx
}

// get db from Context
func (c *Context) GetDB() *gorm.DB {
	return c.DB
}
