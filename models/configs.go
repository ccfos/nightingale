package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/runner"
	"github.com/toolkits/pkg/str"
)

type Configs struct {
	Id   int64 `gorm:"primaryKey"`
	Ckey string
	Cval string
}

func (Configs) TableName() string {
	return "configs"
}

// InitSalt generate random salt
func InitSalt(ctx *ctx.Context) {
	val, err := ConfigsGet(ctx, "salt")
	if err != nil {
		log.Fatalln("cannot query salt", err)
	}

	if val != "" {
		return
	}

	content := fmt.Sprintf("%s%d%d%s", runner.Hostname, os.Getpid(), time.Now().UnixNano(), str.RandLetters(6))
	salt := str.MD5(content)
	err = ConfigsSet(ctx, "salt", salt)
	if err != nil {
		log.Fatalln("init salt in mysql", err)
	}
}

func ConfigsGet(ctx *ctx.Context, ckey string) (string, error) {
	if !ctx.IsCenter {
		if !ctx.IsCenter {
			s, err := poster.GetByUrls[string](ctx, "/v1/n9e/config?key="+ckey)
			return s, err
		}
	}

	var lst []string
	err := DB(ctx).Model(&Configs{}).Where("ckey=?", ckey).Pluck("cval", &lst).Error
	if err != nil {
		return "", errors.WithMessage(err, "failed to query configs")
	}

	if len(lst) > 0 {
		return lst[0], nil
	}

	return "", nil
}

func ConfigsSet(ctx *ctx.Context, ckey, cval string) error {
	num, err := Count(DB(ctx).Model(&Configs{}).Where("ckey=?", ckey))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}

	if num == 0 {
		// insert
		err = DB(ctx).Create(&Configs{
			Ckey: ckey,
			Cval: cval,
		}).Error
	} else {
		// update
		err = DB(ctx).Model(&Configs{}).Where("ckey=?", ckey).Update("cval", cval).Error
	}

	return err
}

func ConfigGet(ctx *ctx.Context, id int64) (*Configs, error) {
	var objs []*Configs
	err := DB(ctx).Where("id=?", id).Find(&objs).Error

	if len(objs) == 0 {
		return nil, nil
	}
	return objs[0], err
}

func ConfigsGets(ctx *ctx.Context, prefix string, limit, offset int) ([]*Configs, error) {
	var objs []*Configs
	session := DB(ctx)
	if prefix != "" {
		session = session.Where("ckey like ?", prefix+"%")
	}

	err := session.Order("id desc").Limit(limit).Offset(offset).Find(&objs).Error
	return objs, err
}

func (c *Configs) Add(ctx *ctx.Context) error {
	num, err := Count(DB(ctx).Model(&Configs{}).Where("ckey=?", c.Ckey))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}
	if num > 0 {
		return errors.WithMessage(err, "key is exists")
	}

	// insert
	err = DB(ctx).Create(&Configs{
		Ckey: c.Ckey,
		Cval: c.Cval,
	}).Error
	return err
}

func (c *Configs) Update(ctx *ctx.Context) error {
	num, err := Count(DB(ctx).Model(&Configs{}).Where("id<>? and ckey=?", c.Id, c.Ckey))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}
	if num > 0 {
		return errors.WithMessage(err, "key is exists")
	}

	err = DB(ctx).Model(&Configs{}).Where("id=?", c.Id).Updates(c).Error
	return err
}

func ConfigsDel(ctx *ctx.Context, ids []int64) error {
	return DB(ctx).Where("id in ?", ids).Delete(&Configs{}).Error
}

func ConfigsGetsByKey(ctx *ctx.Context, ckeys []string) (map[string]string, error) {
	var objs []Configs
	err := DB(ctx).Where("ckey in ?", ckeys).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to gets configs")
	}

	count := len(ckeys)
	kvmap := make(map[string]string, count)
	for i := 0; i < count; i++ {
		kvmap[ckeys[i]] = ""
	}

	for i := 0; i < len(objs); i++ {
		kvmap[objs[i].Ckey] = objs[i].Cval
	}

	return kvmap, nil
}
