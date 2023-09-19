package models

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/secu"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

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
	Config_External  = 1 //external type
	Config_Encrypted = 1 //ciphertext
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

// ConfigsGetPlus retrieves the value for the given config key with decryption and macro variable expansion.
//
// Parameters:
// - ctx: Context containing environment info
// - ckey: The config key to lookup
// - rsaConfig: RSA config for decrypting encrypted config values
//
// Returns:
// - string: The decrypted, expanded config value
// - error: Any error encountered
//
// If on edge node, does a remote lookup.
// If config not found, returns empty string.
// If found, tries to decrypt if encrypted and expand any macro variables.
// Returns original value if decryption or macro expansion fails.
func ConfigsGetPlus(ctx *ctx.Context, ckey string, rsaConfig *httpx.RSAConfig) (string, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[string](ctx, "/v1/n9e/config-plus?key="+ckey)
		return s, err
	}

	objs, err := ConfigsSelectByCkey(ctx, ckey)
	if err != nil {
		return "", err
	}

	if len(objs) > 0 {
		//Try to decode macro variables
		var b *bytes.Buffer
		if b, err = ConfigsMacroVariablesVerify(ctx, objs[0], rsaConfig); err != nil {
			if !errors.As(err, &MacroVariablesNotFoundError{objs[0].Cval}) {
				return objs[0].Cval, err
			} else {
				return decodeConfig(objs[0], rsaConfig) // no macro variables
			}
		}
		return b.String(), nil
	}
	return "", nil
}

func decodeConfig(objs Configs, rsaConfig *httpx.RSAConfig) (string, error) {
	if objs.Encrypted == Config_Encrypted && rsaConfig != nil && rsaConfig.OpenConfigRSA {
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

func ConfigsSetPlus(ctx *ctx.Context, conf Configs, rsaConfig *httpx.RSAConfig, isInternal ...bool) error {
	if len(isInternal) < 1 || !isInternal[0] { //conf.External change default value to Config_External
		conf.External = Config_External
	}
	objs, err := ConfigsSelectByCkey(ctx, conf.Ckey)
	if err != nil {
		return err
	}
	//Before inserting external conf, check if they are already defined as built-in conf
	if conf.External == Config_External && len(objs) == 0 && conf.IsInternal() {
		return fmt.Errorf("duplicate ckey(internal) value found: %s", conf.Ckey)
	}
	//Try to decode macro variables (config with maro variable must not encrypt)
	if _, err = ConfigsMacroVariablesVerify(ctx, conf, rsaConfig); err != nil {
		if !errors.As(err, &MacroVariablesNotFoundError{conf.Cval}) {
			return err
		}
		err = nil
	}
	if len(objs) == 0 { // insert
		conf.Id = 0
		err = DB(ctx).Create(&conf).Error
	} else { // update

		err = DB(ctx).Model(&Configs{Id: objs[0].Id}).Select("cval", "note", "external", "encrypted").Updates(conf).Error
	}
	return err
}

func ConfigsSelectByCkey(ctx *ctx.Context, ckey string) ([]Configs, error) {
	var objs []Configs
	err := DB(ctx).Where("ckey=?", ckey).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to select conf")
	}
	return objs, err
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

func ConfigsGetsByKey(ctx *ctx.Context, ckeys []string, rsa *httpx.RSAConfig) (map[string]string, error) {
	var objs []Configs
	err := DB(ctx).Where("ckey in ?", ckeys).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to gets configs")
	}

	count := len(objs)
	kvmap := make(map[string]string, count)
	for i := 0; i < len(objs); i++ {
		configCval, err := decodeConfig(objs[i], rsa)
		if err != nil {
			return nil, errors.WithMessage(err, "failed in ConfigsGetsByKey,ckey= "+objs[i].Ckey)
		}
		kvmap[objs[i].Ckey] = configCval
	}

	return kvmap, nil
}

type MacroVariablesNotFoundError struct {
	Cval string
}

func (e MacroVariablesNotFoundError) Error() string {
	return fmt.Sprintf("unable to find any macro variables for '%s'", e.Cval)
}

func (c *Configs) IsInternal() bool {
	return slice.ContainsString(INTERNAL_CKEY_SLICE, c.Ckey)
}

func ConfigsMacroVariablesVerify(ctx *ctx.Context, f Configs, rsa *httpx.RSAConfig) (b *bytes.Buffer, err error) {
	variables := tplx.ExtractTemplateVariables(f.Cval) //secret keys
	if len(variables) < 1 {
		return nil, MacroVariablesNotFoundError{f.Cval}
	}
	var macroMap map[string]string
	macroMap, err = ConfigsGetsByKey(ctx, variables, rsa)
	if err != nil {
		return nil, err
	}
	if len(macroMap) != len(variables) {
		return nil, fmt.Errorf("missing required macro variable configurations. need %v configurations, found %v", len(variables), len(macroMap))
	}
	b, err = tplx.ReplaceMacroVariables(f.Ckey, f.Cval, macroMap) //try to decode secret
	return
}

func ConfigsGetUserVariable(context *ctx.Context, query string) ([]Configs, error) {
	var objs []Configs
	tx := DB(context).Where("external = ?", Config_External)
	if "" != query {
		q := "%" + query + "%"
		tx.Where("ckey like ? or note like ?", q, q)
	}
	err := tx.Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to gets user variable")
	}

	return objs, nil
}
