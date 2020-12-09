package models

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

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
	has, err := DB["rdb"].Where("ckey=?", ckey).Get(&obj)
	if err != nil {
		return "", err
	}

	if !has {
		return "", nil
	}

	return obj.Cval, nil
}

func ConfigsSet(ckey, cval string) error {
	var obj Configs
	has, err := DB["rdb"].Where("ckey=?", ckey).Get(&obj)
	if err != nil {
		return err
	}

	if !has {
		_, err = DB["rdb"].Insert(Configs{
			Ckey: ckey,
			Cval: cval,
		})
	} else {
		obj.Cval = cval
		_, err = DB["rdb"].Where("ckey=?", ckey).Cols("cval").Update(obj)
	}

	return err
}

func ConfigsGets(ckeys []string) (map[string]string, error) {
	var objs []Configs
	err := DB["rdb"].In("ckey", ckeys).Find(&objs)
	if err != nil {
		return nil, err
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

type AuthConfig struct {
	MaxNumErr          int   `json:"maxNumErr"`
	MaxSessionNumber   int64 `json:"maxSessionNumber"`
	MaxConnIdelTime    int64 `json:"maxConnIdelTime"`
	LockTime           int64 `json:"lockTime"`
	PwdHistorySize     int   `json:"pwdHistorySize"`
	PwdMinLenght       int   `json:"pwdMinLenght"`
	PwdExpiresIn       int64 `json:"pwdExpiresIn"`
	PwdIncludeUpper    int   `json:"pwdIncludeUpper" description:"'0': fasle, other: true"`
	PwdIncludeLower    int   `json:"pwdIncludeLower"`
	PwdIncludeNumber   int   `json:"pwdIncludeNumber"`
	PwdIncludeSpecChar int   `json:"pwdIncludeSpecChar"`
}

func AuthConfigGet() (*AuthConfig, error) {
	buf, err := ConfigsGet("auth.config")
	if err != nil {
		return nil, err
	}
	config := &AuthConfig{}
	if err := json.Unmarshal([]byte(buf), config); err != nil {
		return nil, err
	}
	return config, nil
}

func AuthConfigSet(config *AuthConfig) error {
	buf, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return ConfigsSet("auth.config", string(buf))
}
