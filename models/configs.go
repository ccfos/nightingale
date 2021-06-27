package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
	"github.com/toolkits/pkg/str"
)

type Configs struct {
	Id   int64
	Ckey string
	Cval string
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
	var obj Configs
	has, err := DB.Where("ckey=?", ckey).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query configs(ckey=%s) fail: %v", ckey, err)
		return "", internalServerError
	}

	if !has {
		return "", nil
	}

	return obj.Cval, nil
}

func ConfigsSet(ckey, cval string) error {
	var obj Configs
	has, err := DB.Where("ckey=?", ckey).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: get configs(ckey=%s) fail: %v", ckey, err)
		return internalServerError
	}

	if !has {
		_, err = DB.Insert(Configs{
			Ckey: ckey,
			Cval: cval,
		})
	} else {
		obj.Cval = cval
		_, err = DB.Where("ckey=?", ckey).Cols("cval").Update(obj)
	}

	if err != nil {
		logger.Errorf("mysql.error: set configs(ckey=%s, cval=%s) fail: %v", ckey, cval, err)
		return internalServerError
	}

	return nil
}

func ConfigsGets(ckeys []string) (map[string]string, error) {
	var objs []Configs
	err := DB.In("ckey", ckeys).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: gets configs fail: %v", err)
		return nil, internalServerError
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
