package ctx

import (
	"context"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/ccfos/nightingale/v6/conf"
	"gorm.io/gorm"
)

type Context struct {
	DB        *gorm.DB
	Redis     storage.Redis
	CenterApi conf.CenterApi
	Ctx       context.Context
	IsCenter  bool
}

func NewContext(ctx context.Context, db *gorm.DB, redis storage.Redis, isCenter bool, centerApis ...conf.CenterApi) *Context {
	var api conf.CenterApi
	if len(centerApis) > 0 {
		api = centerApis[0]
	}

	return &Context{
		Ctx:       ctx,
		DB:        db,
		Redis:     redis,
		CenterApi: api,
		IsCenter:  isCenter,
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
