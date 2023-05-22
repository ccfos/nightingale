package models

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type Datasource struct {
	Id             int64                  `json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	PluginId       int64                  `json:"plugin_id"`
	PluginType     string                 `json:"plugin_type"`      // prometheus
	PluginTypeName string                 `json:"plugin_type_name"` // Prometheus Like
	Category       string                 `json:"category"`         // timeseries
	ClusterName    string                 `json:"cluster_name"`
	Settings       string                 `json:"-" gorm:"settings"`
	SettingsJson   map[string]interface{} `json:"settings" gorm:"-"`
	Status         string                 `json:"status"`
	HTTP           string                 `json:"-" gorm:"http"`
	HTTPJson       HTTP                   `json:"http" gorm:"-"`
	Auth           string                 `json:"-" gorm:"auth"`
	AuthJson       Auth                   `json:"auth" gorm:"-"`
	CreatedAt      int64                  `json:"created_at"`
	UpdatedAt      int64                  `json:"updated_at"`
	CreatedBy      string                 `json:"created_by"`
	UpdatedBy      string                 `json:"updated_by"`
	Transport      *http.Transport        `json:"-" gorm:"-"`
}

type Auth struct {
	BasicAuth         bool   `json:"basic_auth"`
	BasicAuthUser     string `json:"basic_auth_user"`
	BasicAuthPassword string `json:"basic_auth_password"`
}

type HTTP struct {
	Timeout             int64             `json:"timeout"`
	DialTimeout         int64             `json:"dial_timeout"`
	TLS                 TLS               `json:"tls"`
	MaxIdleConnsPerHost int               `json:"max_idle_conns_per_host"`
	Url                 string            `json:"url"`
	Headers             map[string]string `json:"headers"`
}

type TLS struct {
	SkipTlsVerify bool `json:"skip_tls_verify"`
}

func (ds *Datasource) TableName() string {
	return "datasource"
}

func (ds *Datasource) Verify() error {
	if str.Dangerous(ds.Name) {
		return errors.New("Name has invalid characters")
	}

	err := ds.FE2DB()
	return err
}

func (ds *Datasource) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if err := ds.Verify(); err != nil {
		return err
	}

	ds.UpdatedAt = time.Now().Unix()
	return DB(ctx).Model(ds).Select(selectField, selectFields...).Updates(ds).Error
}

func (ds *Datasource) Add(ctx *ctx.Context) error {
	if err := ds.Verify(); err != nil {
		return err
	}

	now := time.Now().Unix()
	ds.CreatedAt = now
	ds.UpdatedAt = now
	return Insert(ctx, ds)
}

func DatasourceDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(Datasource)).Error
}

func DatasourceGet(ctx *ctx.Context, id int64) (*Datasource, error) {
	var ds *Datasource
	err := DB(ctx).Where("id = ?", id).First(&ds).Error
	if err != nil {
		return nil, err
	}
	return ds, ds.DB2FE()
}

func (ds *Datasource) Get(ctx *ctx.Context) error {
	err := DB(ctx).Where("id = ?", ds.Id).First(ds).Error
	if err != nil {
		return err
	}
	return ds.DB2FE()
}

func GetDatasources(ctx *ctx.Context) ([]Datasource, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]Datasource](ctx, "/v1/n9e/datasources")
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(lst); i++ {
			lst[i].FE2DB()
		}
		return lst, nil
	}

	var dss []Datasource
	err := DB(ctx).Find(&dss).Error

	for i := 0; i < len(dss); i++ {
		dss[i].DB2FE()
	}

	return dss, err
}

func GetDatasourceIdsByEngineName(ctx *ctx.Context, engineName string) ([]int64, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]int64](ctx, "/v1/n9e/datasource-ids?name="+engineName)
		return lst, err
	}

	var dss []Datasource
	var ids []int64
	err := DB(ctx).Where("cluster_name = ?", engineName).Find(&dss).Error
	if err != nil {
		return ids, err
	}

	for i := 0; i < len(dss); i++ {
		ids = append(ids, dss[i].Id)
	}
	return ids, err
}

func GetDatasourcesCountBy(ctx *ctx.Context, typ, cate, name string) (int64, error) {
	session := DB(ctx).Model(&Datasource{})

	if name != "" {
		arr := strings.Fields(name)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("name =  ?", qarg)
		}
	}

	if typ != "" {
		session = session.Where("plugin_type = ?", typ)
	}

	if cate != "" {
		session = session.Where("category = ?", cate)
	}

	return Count(session)
}

func GetDatasourcesGetsBy(ctx *ctx.Context, typ, cate, name, status string) ([]*Datasource, error) {
	session := DB(ctx)

	if name != "" {
		arr := strings.Fields(name)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("name =  ?", qarg)
		}
	}

	if typ != "" {
		session = session.Where("plugin_type = ?", typ)
	}

	if cate != "" {
		session = session.Where("category = ?", cate)
	}

	if status != "" {
		session = session.Where("status = ?", status)
	}

	var lst []*Datasource
	err := session.Order("id desc").Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}
	return lst, err
}

func GetDatasourcesGetsByTypes(ctx *ctx.Context, typs []string) (map[string]*Datasource, error) {
	var lst []*Datasource
	m := make(map[string]*Datasource)
	err := DB(ctx).Where("plugin_type in ?", typs).Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
			m[lst[i].Name] = lst[i]
		}
	}
	return m, err
}

func (ds *Datasource) FE2DB() error {
	if ds.SettingsJson != nil {
		b, err := json.Marshal(ds.SettingsJson)
		if err != nil {
			return err
		}
		ds.Settings = string(b)
	}

	b, err := json.Marshal(ds.HTTPJson)
	if err != nil {
		return err
	}
	ds.HTTP = string(b)

	b, err = json.Marshal(ds.AuthJson)
	if err != nil {
		return err
	}
	ds.Auth = string(b)

	return nil
}

func (ds *Datasource) DB2FE() error {
	if ds.Settings != "" {
		err := json.Unmarshal([]byte(ds.Settings), &ds.SettingsJson)
		if err != nil {
			return err
		}
	}

	if ds.HTTP != "" {
		err := json.Unmarshal([]byte(ds.HTTP), &ds.HTTPJson)
		if err != nil {
			return err
		}
	}

	if ds.HTTPJson.Timeout == 0 {
		ds.HTTPJson.Timeout = 10000
	}

	if ds.HTTPJson.DialTimeout == 0 {
		ds.HTTPJson.DialTimeout = 10000
	}

	if ds.HTTPJson.MaxIdleConnsPerHost == 0 {
		ds.HTTPJson.MaxIdleConnsPerHost = 100
	}

	if ds.Auth != "" {
		err := json.Unmarshal([]byte(ds.Auth), &ds.AuthJson)
		if err != nil {
			return err
		}
	}

	return nil
}

func DatasourceGetMap(ctx *ctx.Context) (map[int64]*Datasource, error) {
	var lst []*Datasource
	var err error
	if !ctx.IsCenter {
		lst, err = poster.GetByUrls[[]*Datasource](ctx, "/v1/n9e/datasources")
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(lst); i++ {
			lst[i].FE2DB()
		}
	} else {
		err := DB(ctx).Find(&lst).Error
		if err != nil {
			return nil, err
		}

		for i := 0; i < len(lst); i++ {
			err := lst[i].DB2FE()
			if err != nil {
				logger.Warningf("get ds:%+v err:%v", lst[i], err)
				continue
			}
		}
	}

	ret := make(map[int64]*Datasource)
	for i := 0; i < len(lst); i++ {
		ret[lst[i].Id] = lst[i]
	}

	return ret, nil
}

func DatasourceStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=datasource")
		return s, err
	}

	session := DB(ctx).Model(&Datasource{}).Select("count(*) as total", "max(updated_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}
