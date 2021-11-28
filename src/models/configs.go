package models

import (
	"fmt"
	"log"
	"os"
	"time"

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
func InitSalt() {
	val, err := ConfigsGet("salt")
	if err != nil {
		log.Fatalln("cannot query salt", err)
	}

	if val != "" {
		return
	}

	content := fmt.Sprintf("%s%d%d%s", runner.Hostname, os.Getpid(), time.Now().UnixNano(), str.RandLetters(6))
	salt := str.MD5(content)
	err = ConfigsSet("salt", salt)
	if err != nil {
		log.Fatalln("init salt in mysql", err)
	}
}

func ConfigsGet(ckey string) (string, error) {
	var lst []string
	err := DB().Model(&Configs{}).Where("ckey=?", ckey).Pluck("cval", &lst).Error
	if err != nil {
		return "", errors.WithMessage(err, "failed to query configs")
	}

	if len(lst) > 0 {
		return lst[0], nil
	}

	return "", nil
}

func ConfigsSet(ckey, cval string) error {
	num, err := Count(DB().Model(&Configs{}).Where("ckey=?", ckey))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}

	if num == 0 {
		// insert
		err = DB().Create(&Configs{
			Ckey: ckey,
			Cval: cval,
		}).Error
	} else {
		// update
		err = DB().Model(&Configs{}).Where("ckey=?", ckey).Update("cval", cval).Error
	}

	return err
}

func ConfigsGets(ckeys []string) (map[string]string, error) {
	var objs []Configs
	err := DB().Where("ckey in ?", ckeys).Find(&objs).Error
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
