package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/runner"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
)

type Configs struct {
	Id        int64  `gorm:"primaryKey"`
	Ckey      string //Unique field. Before inserting external configs, check if they are already defined as built-in configs.
	Cval      string
	Note      string
	External  int //Controls frontend list display: 0 hides built-in (default), 1 shows external
	Encrypted int //Indicates whether the value(cval) is encrypted (1 for ciphertext, 0 for plaintext(default))
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

func decodeConfig(objs Configs, rsaConfig *httpx.RSAConfig) (string, error) {
	if objs.Encrypted == ConfigEncrypted && rsaConfig != nil && rsaConfig.OpenConfigRSA {
		decrypted, err := secu.Decrypt(objs.Cval, rsaConfig.RSAPrivateKey, rsaConfig.RSAPassWord)
		if err != nil {
			return objs.Cval, fmt.Errorf("failed to decode config (%+v),error info %s", objs, err.Error())
		}
		objs.Cval = decrypted
	}
	return objs.Cval, nil
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
		fmt.Errorf("duplicate ckey found: %s", conf.Ckey)
	}
	return DB(context).Create(&conf).Error
}

func ConfigsUserVariableUpdate(context *ctx.Context, conf Configs) error {
	conf.External = ConfigExternal
	if conf.IsInternal() {
		return fmt.Errorf("duplicate ckey(internal) value found: %s", conf.Ckey)
	}
	obj, err := ConfigGet(context, conf.Id)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("not found ckey: %s", conf.Ckey)
	}
	if obj.Id != conf.Id {
		return fmt.Errorf("duplicate ckey(external) value found: %s", conf.Ckey)
	}
	return DB(context).Model(&Configs{Id: obj.Id}).Select("ckey", "cval", "note", "external", "encrypted").Updates(conf).Error
}
