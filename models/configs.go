package models

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
)

type Configs struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	Ckey      string `json:"ckey"` //Unique field. Before inserting external configs, check if they are already defined as built-in configs.
	Cval      string `json:"cval"`
	Note      string `json:"note"`
	External  int    `json:"external"`  //Controls frontend list display: 0 hides built-in (default), 1 shows external
	Encrypted int    `json:"encrypted"` //Indicates whether the value(cval) is encrypted (1 for ciphertext, 0 for plaintext(default))
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

const SALT = "salt"

// InitSalt generate random salt
func InitSalt(ctx *ctx.Context) {
	val, err := ConfigsGet(ctx, SALT)
	if err != nil {
		log.Fatalln("cannot query salt", err)
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
	num, err := Count(DB(ctx).Model(&Configs{}).Where("ckey=?", c.Ckey))
	if err != nil {
		return errors.WithMessage(err, "failed to count configs")
	}
	if num > 0 {
		return errors.New("key is exists")
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
		return errors.New("key is exists")
	}

	err = DB(ctx).Model(&Configs{}).Where("id=?", c.Id).Updates(c).Error
	return err
}

func ConfigsDel(ctx *ctx.Context, ids []int64) error {
	return DB(ctx).Where("id in ?", ids).Delete(&Configs{}).Error
}

func (c *Configs) IsInternal() bool {
	return slice.ContainsString(InternalCkeySlice, c.Ckey)
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
	//Before inserting external conf, check if they are already defined as built-in conf
	if conf.IsInternal() {
		return fmt.Errorf("duplicate ckey(internal) value found: %s", conf.Ckey)
	}
	objs, err := ConfigsSelectByCkey(context, conf.Ckey)
	if err != nil {
		return err
	}
	if len(objs) > 0 {
		return fmt.Errorf("duplicate ckey found: %s", conf.Ckey)
	}
	return DB(context).Create(&conf).Error
}

func ConfigsUserVariableUpdate(context *ctx.Context, conf Configs) error {
	if conf.IsInternal() {
		return fmt.Errorf("duplicate ckey(internal) value found: %s", conf.Ckey)
	}
	err := userVariableCheck(context, conf)
	if err != nil {
		return err
	}
	return DB(context).Model(&Configs{Id: conf.Id}).Select("ckey", "cval", "note", "encrypted").Updates(conf).Error
}

func userVariableCheck(context *ctx.Context, conf Configs) error {
	var objs []*Configs
	// id and ckey both unique
	err := DB(context).Where("id <> ? and ckey = ? ", conf.Id, conf.Ckey).Find(&objs).Error
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return nil
	}
	return fmt.Errorf("duplicate ckey value found: %s", conf.Ckey)
}

func ConfigsUserVariableStatistics(context *ctx.Context) (*Statistics, error) {
	if !context.IsCenter {
		return poster.GetByUrls[*Statistics](context, "/v1/n9e/statistic?name=user_variable")
	}

	session := DB(context).Model(&Configs{}).Select(
		"count(*) as total", "1 as last_updated").Where("external = ?", ConfigExternal)

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats[0], nil
}

func MacroVariableGetDecryptMap(context *ctx.Context, privateKey []byte, passWord string) (map[string]string, error) {

	if !context.IsCenter {
		ret, err := poster.GetByUrls[map[string]string](context, "/v1/n9e/macro-variable")
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

func ConfigsGetDecryption(cvalFun func() (string, string, error), macroMap map[string]string) (string, error) {
	ckey, cval, err := cvalFun()
	if err != nil {
		return "", errors.WithMessage(err, "failed to gets ConfigsGetDecryption.")
	}
	if strings.TrimSpace(cval) == "" {
		return cval, nil
	}
	tplxBuffer, replaceErr := tplx.ReplaceMacroVariables(ckey, cval, macroMap)
	if replaceErr != nil {
		return "", errors.WithMessage(replaceErr, "failed to gets ConfigsGetDecryption. ReplaceMacroVariables error.")
	}
	if tplxBuffer != nil {
		return tplxBuffer.String(), nil
	}
	return "", fmt.Errorf("unexpected error. ckey:%s, cval:%s, macroMap:%+v,tplxBuffer(pointer):%v", ckey, cval, macroMap, tplxBuffer)
}
