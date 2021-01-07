package models

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
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
	MaxNumErr          int      `json:"maxNumErr"`
	MaxSessionNumber   int64    `json:"maxSessionNumber"`
	MaxConnIdleTime    int64    `json:"maxConnIdleTime" description:"minute"`
	LockTime           int64    `json:"lockTime" description:"minute"`
	PwdHistorySize     int      `json:"pwdHistorySize"`
	PwdMinLenght       int      `json:"pwdMinLenght"`
	PwdExpiresIn       int64    `json:"pwdExpiresIn" description:"month"`
	PwdMustInclude     []string `json:"pwdMustInclude" description:"upper,lower,number,specChar"`
	PwdMustIncludeFlag int      `json:"pwdMustIncludeFlag"`
}

func (p AuthConfig) PwdRules() []string {
	s := []string{}
	if p.PwdMinLenght > 0 {
		s = append(s, _s("Minimum password length %d", p.PwdMinLenght))
	}
	if rule := p.MustInclude(); rule != "" {
		s = append(s, rule)
	}
	return s
}

func (p AuthConfig) MustInclude() string {
	s := []string{}
	if p.PwdMustIncludeFlag&PWD_INCLUDE_UPPER > 0 {
		s = append(s, _s("Upper char"))
	}
	if p.PwdMustIncludeFlag&PWD_INCLUDE_LOWER > 0 {
		s = append(s, _s("Lower char"))
	}
	if p.PwdMustIncludeFlag&PWD_INCLUDE_NUMBER > 0 {
		s = append(s, _s("Number"))
	}
	if p.PwdMustIncludeFlag&PWD_INCLUDE_SPEC_CHAR > 0 {
		s = append(s, _s("Special char"))
	}

	if len(s) > 0 {
		return _s("Must include %s", strings.Join(s, ","))
	}

	return ""
}

const (
	PWD_INCLUDE_UPPER = 1 << iota
	PWD_INCLUDE_LOWER
	PWD_INCLUDE_NUMBER
	PWD_INCLUDE_SPEC_CHAR
)

func (p *AuthConfig) Validate() error {
	for _, v := range p.PwdMustInclude {
		switch v {
		case "upper":
			p.PwdMustIncludeFlag |= PWD_INCLUDE_UPPER
		case "lower":
			p.PwdMustIncludeFlag |= PWD_INCLUDE_LOWER
		case "number":
			p.PwdMustIncludeFlag |= PWD_INCLUDE_NUMBER
		case "specChar":
			p.PwdMustIncludeFlag |= PWD_INCLUDE_SPEC_CHAR
		default:
			return fmt.Errorf("invalid pwd flags, must be 'upper', 'lower', 'number', 'specChar'")
		}
	}

	return nil
}

var DefaultAuthConfig = AuthConfig{
	MaxConnIdleTime: 30,
	PwdMustInclude:  []string{},
}

func AuthConfigGet() (*AuthConfig, error) {
	buf, err := ConfigsGet("auth.config")
	if err != nil {
		return &DefaultAuthConfig, nil
	}
	c := &AuthConfig{}
	if err := json.Unmarshal([]byte(buf), c); err != nil {
		return &DefaultAuthConfig, nil
	}
	return c, nil
}

func AuthConfigSet(config *AuthConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	buf, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return ConfigsSet("auth.config", string(buf))
}
