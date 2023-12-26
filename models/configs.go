package models

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
	"github.com/toolkits/pkg/str"
)

type Configs struct { //ckey+external
	Id        int64  `json:"id" gorm:"primaryKey"`
	Ckey      string `json:"ckey"` // Before inserting external configs, check if they are already defined as built-in configs.
	Cval      string `json:"cval"`
	Note      string `json:"note"`
	External  int    `json:"external"`  //Controls frontend list display: 0 hides built-in (default), 1 shows external
	Encrypted int    `json:"encrypted"` //Indicates whether the value(cval) is encrypted (1 for ciphertext, 0 for plaintext(default))
	CreateAt  int64  `json:"create_at"`
	CreateBy  string `json:"create_by"`
	UpdateAt  int64  `json:"update_at"`
	UpdateBy  string `json:"update_by"`
}

func (Configs) TableName() string {
	return "configs"
}

var (
	ConfigExternal  = 1 //external type
	ConfigEncrypted = 1 //ciphertext
)

func (c *Configs) DB2FE() error {
	return nil
}

const (
	SALT            = "salt"
	RSA_PRIVATE_KEY = "rsa_private_key"
	RSA_PUBLIC_KEY  = "rsa_public_key"
	RSA_PASSWORD    = "rsa_password"
)

// InitSalt generate random salt
func InitSalt(ctx *ctx.Context) {
	val, err := ConfigsGet(ctx, SALT)
	if err != nil {
		log.Fatalln("init salt in mysql", err)
	}

	if val != "" {
		return
	}

	content := fmt.Sprintf("%s%d%d%s", runner.Hostname, os.Getpid(), time.Now().UnixNano(), str.RandLetters(6))
	salt := str.MD5(content)
	err = ConfigsSet(ctx, SALT, salt)
	if err != nil {
		log.Fatalln("init salt in mysql", err)
	}

}
func InitRSAPassWord(ctx *ctx.Context) (string, error) {

	val, err := ConfigsGet(ctx, RSA_PASSWORD)
	if err != nil {
		return "", errors.WithMessage(err, "failed to get rsa password")
	}
	if val != "" {
		return val, nil
	}
	content := fmt.Sprintf("%s%d%d%s", runner.Hostname, os.Getpid(), time.Now().UnixNano(), str.RandLetters(6))
	pwd := str.MD5(content)
	err = ConfigsSet(ctx, RSA_PASSWORD, pwd)
	if err != nil {
		return "", errors.WithMessage(err, "failed to set rsa password")
	}
	return pwd, nil
}

func ConfigsGet(ctx *ctx.Context, ckey string) (string, error) { //select built-in type configs
	if !ctx.IsCenter {
		if !ctx.IsCenter {
			s, err := poster.GetByUrls[string](ctx, "/v1/n9e/config?key="+ckey)
			return s, err
		}
	}

	var lst []string
	err := DB(ctx).Model(&Configs{}).Where("ckey=?  and external=? ", ckey, 0).Pluck("cval", &lst).Error
	if err != nil {
		return "", errors.WithMessage(err, "failed to query configs")
	}

	if len(lst) > 0 {
		return lst[0], nil
	}

	return "", nil
}

func ConfigsSet(ctx *ctx.Context, ckey, cval string) error {
	return ConfigsSetWithUname(ctx, ckey, cval, "default")
}
func ConfigsSetWithUname(ctx *ctx.Context, ckey, cval, uName string) error { //built-in
	num, err := Count(DB(ctx).Model(&Configs{}).Where("ckey=? and external=?", ckey, 0)) //built-in type
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}
	now := time.Now().Unix()
	if num == 0 {
		// insert
		err = DB(ctx).Create(&Configs{
			Ckey:     ckey,
			Cval:     cval,
			CreateBy: uName,
			UpdateBy: uName,
			CreateAt: now,
			UpdateAt: now,
		}).Error
	} else {
		// update
		err = DB(ctx).Model(&Configs{}).Where("ckey=?", ckey).Updates(map[string]interface{}{
			"cval":      cval,
			"update_by": uName,
			"update_at": now,
		}).Error
	}

	return err
}

func ConfigsSelectByCkey(ctx *ctx.Context, ckey string) ([]Configs, error) {
	var objs []Configs
	err := DB(ctx).Where("ckey=?", ckey).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to select conf")
	}
	return objs, nil
}

func ConfigGet(ctx *ctx.Context, id int64) (*Configs, error) {
	var objs []*Configs
	err := DB(ctx).Where("id=?", id).Find(&objs).Error
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, nil
	}
	return objs[0], nil
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
	num, err := Count(DB(ctx).Model(&Configs{}).Where("ckey=? and external=? ", c.Ckey, c.External))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}
	if num > 0 {
		return errors.New("key is exists")
	}

	// insert
	err = DB(ctx).Create(&Configs{
		Ckey:     c.Ckey,
		Cval:     c.Cval,
		External: c.External,
		CreateBy: c.CreateBy,
		UpdateBy: c.CreateBy,
		CreateAt: c.CreateAt,
		UpdateAt: c.CreateAt,
	}).Error
	return err
}

func (c *Configs) Update(ctx *ctx.Context) error {
	num, err := Count(DB(ctx).Model(&Configs{}).Where("id<>? and ckey=? and external=? ", c.Id, c.Ckey, c.External))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}
	if num > 0 {
		return errors.New("key is exists")
	}
	err = DB(ctx).Model(&Configs{}).Where("id=?", c.Id).Updates(c).Error
	return err
}

func ConfigsDel(ctx *ctx.Context, ids []int64) error {
	return DB(ctx).Where("id in ?", ids).Delete(&Configs{}).Error
}

func ConfigsGetUserVariable(context *ctx.Context) ([]Configs, error) {
	var objs []Configs
	tx := DB(context).Where("external = ?", ConfigExternal).Order("id desc")
	err := tx.Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to gets user variable")
	}

	return objs, nil
}

func ConfigsUserVariableInsert(context *ctx.Context, conf Configs) error {
	conf.External = ConfigExternal
	conf.Id = 0
	err := userVariableCheck(context, conf.Ckey, conf.Id)
	if err != nil {
		return err
	}

	return DB(context).Create(&conf).Error
}

func ConfigsUserVariableUpdate(context *ctx.Context, conf Configs) error {
	err := userVariableCheck(context, conf.Ckey, conf.Id)
	if err != nil {
		return err
	}
	configOld, _ := ConfigGet(context, conf.Id)
	if configOld == nil || configOld.External != ConfigExternal { //not valid id
		return fmt.Errorf("not valid configs(id)")
	}
	return DB(context).Model(&Configs{Id: conf.Id}).Select(
		"ckey", "cval", "note", "encrypted", "update_by", "update_at").Updates(conf).Error
}

func isCStyleIdentifier(str string) bool {
	regex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	return regex.MatchString(str)
}

func userVariableCheck(context *ctx.Context, ckey string, id int64) error {
	var objs []*Configs
	var err error
	if !isCStyleIdentifier(ckey) {
		return fmt.Errorf("invalid key(%q), please use ^[a-zA-Z_][a-zA-Z0-9_]*$ ", ckey)
	}

	//  reserved words
	words := []string{"Scheme", "Host", "Hostname", "Port", "Path", "Query", "Fragment"}
	for _, word := range words {
		if ckey == word {
			return fmt.Errorf("invalid key(%q), reserved words, please use other key", ckey)
		}
	}

	if id != 0 { //update
		err = DB(context).Where("id <> ? and ckey = ? and external=?", &id, ckey, ConfigExternal).Find(&objs).Error
	} else {
		err = DB(context).Where("ckey = ? and external=?", ckey, ConfigExternal).Find(&objs).Error
	}
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return nil
	}
	return fmt.Errorf("duplicate ckey value found: %s", ckey)
}

func ConfigsUserVariableStatistics(context *ctx.Context) (*Statistics, error) {
	if !context.IsCenter {
		return poster.GetByUrls[*Statistics](context, "/v1/n9e/statistic?name=user_variable")
	}

	session := DB(context).Model(&Configs{}).Select(
		"count(*) as total", "max(update_at) as last_updated").Where("external = ?", ConfigExternal)

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats[0], nil
}

func ConfigUserVariableGetDecryptMap(context *ctx.Context, privateKey []byte, passWord string) (map[string]string, error) {

	if !context.IsCenter {
		ret, err := poster.GetByUrls[map[string]string](context, "/v1/n9e/user-variable/decrypt")
		if err != nil {
			return nil, err
		}
		return ret, nil
	}
	lst, err := ConfigsGetUserVariable(context)
	if err != nil {
		return nil, err
	}
	ret := make(map[string]string, len(lst))
	for i := 0; i < len(lst); i++ {
		if lst[i].Encrypted != ConfigEncrypted {
			ret[lst[i].Ckey] = lst[i].Cval
		} else {
			decCval, decErr := secu.Decrypt(lst[i].Cval, privateKey, passWord)
			if decErr != nil {
				logger.Errorf("RSA Decrypt failed: %v. Ckey: %s", decErr, lst[i].Ckey)
				decCval = ""
			}
			ret[lst[i].Ckey] = decCval
		}
	}

	return ret, nil
}
