package models

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type Datasource struct {
	Id              int64                  `json:"id"`
	Name            string                 `json:"name"`
	Identifier      string                 `json:"identifier"`
	Description     string                 `json:"description"`
	PluginId        int64                  `json:"plugin_id"`
	PluginType      string                 `json:"plugin_type"`      // prometheus
	PluginTypeName  string                 `json:"plugin_type_name"` // Prometheus Like
	Category        string                 `json:"category"`         // timeseries
	ClusterName     string                 `json:"cluster_name"`
	Settings        string                 `json:"-" gorm:"settings"`
	SettingsJson    map[string]interface{} `json:"settings" gorm:"-"`
	SettingsEncoded string                 `json:"settings_encoded" gorm:"-"`
	Status          string                 `json:"status"`
	HTTP            string                 `json:"-" gorm:"http"`
	HTTPJson        HTTP                   `json:"http" gorm:"-"`
	Auth            string                 `json:"-" gorm:"auth"`
	AuthJson        Auth                   `json:"auth" gorm:"-"`
	AuthEncoded     string                 `json:"auth_encoded" gorm:"-"`
	CreatedAt       int64                  `json:"created_at"`
	UpdatedAt       int64                  `json:"updated_at"`
	CreatedBy       string                 `json:"created_by"`
	UpdatedBy       string                 `json:"updated_by"`
	IsDefault       bool                   `json:"is_default"`
	Weight          int                    `json:"weight"`
	Transport       *http.Transport        `json:"-" gorm:"-"`
	ForceSave       bool                   `json:"force_save" gorm:"-"`
}

type Auth struct {
	BasicAuth         bool   `json:"basic_auth"`
	BasicAuthUser     string `json:"basic_auth_user"`
	BasicAuthPassword string `json:"basic_auth_password"`
}

var rsaConfig *RsaConfig

type RsaConfig struct {
	OpenRSA         bool   `json:"open_rsa"`
	RSAPublicKey    string `json:"rsa_public_key,omitempty"`
	RSAPrivateKey   string `json:"rsa_private_key,omitempty"`
	RSAPassWord     string `json:"rsa_password,omitempty"`
	PrivateKeyBytes []byte
}

func SetRsaConfig(cfg *RsaConfig) {
	if cfg != nil {
		rsaConfig = cfg
		return
	}
	logger.Warning("Rsa config is nil")
}

func GetRsaConfig() *RsaConfig {
	return rsaConfig
}

type HTTP struct {
	Timeout             int64             `json:"timeout"`
	DialTimeout         int64             `json:"dial_timeout"`
	TLS                 TLS               `json:"tls"`
	MaxIdleConnsPerHost int               `json:"max_idle_conns_per_host"`
	Url                 string            `json:"url"`
	Urls                []string          `json:"urls"`
	Headers             map[string]string `json:"headers"`
}

func (h HTTP) IsLoki() bool {
	if strings.Contains(h.Url, "loki") {
		return true
	}

	for k := range h.Headers {
		tmp := strings.ToLower(k)
		if strings.Contains(tmp, "loki") {
			return true
		}
	}

	return false
}

func (h HTTP) GetUrls() []string {
	var urls []string
	if len(h.Urls) == 0 {
		urls = []string{h.Url}
	} else {
		// 复制切片以避免修改原始数据
		urls = make([]string, len(h.Urls))
		copy(urls, h.Urls)
	}

	// 使用 Fisher-Yates 洗牌算法随机打乱顺序
	for i := len(urls) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		urls[i], urls[j] = urls[j], urls[i]
	}

	return urls
}

func (h HTTP) NewReq(reqUrl *string) (req *http.Request, err error) {
	urls := h.GetUrls()
	for i := 0; i < len(urls); i++ {
		if req, err = http.NewRequest("GET", urls[i], nil); err == nil {
			*reqUrl = urls[i]
			return
		}
	}
	return
}

func (h HTTP) ParseUrl() (target *url.URL, err error) {
	urls := h.GetUrls()
	if len(urls) == 0 {
		return nil, errors.New("no urls")
	}

	target, err = url.Parse(urls[0])
	if err != nil {
		return nil, err
	}
	return
}

type TLS struct {
	SkipTlsVerify bool `json:"skip_tls_verify"`
	// mTLS 配置
	CACert            string `json:"ca_cert"`             // CA 证书内容 (PEM 格式)
	ClientCert        string `json:"client_cert"`         // 客户端证书内容 (PEM 格式)
	ClientKey         string `json:"client_key"`          // 客户端密钥内容 (PEM 格式)
	ClientKeyPassword string `json:"client_key_password"` // 密钥密码（可选）
	ServerName        string `json:"server_name"`         // TLS ServerName（可选，用于证书验证）
	MinVersion        string `json:"min_version"`         // TLS 最小版本 (1.0, 1.1, 1.2, 1.3)
	MaxVersion        string `json:"max_version"`         // TLS 最大版本
}

// TLSConfig 从证书内容创建 tls.Config
// 证书内容为 PEM 格式字符串
func (t *TLS) TLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: t.SkipTlsVerify,
	}

	// 设置 ServerName
	if t.ServerName != "" {
		tlsConfig.ServerName = t.ServerName
	}

	// 设置 TLS 版本
	if t.MinVersion != "" {
		if v, ok := tlsVersionMap[t.MinVersion]; ok {
			tlsConfig.MinVersion = v
		}
	}
	if t.MaxVersion != "" {
		if v, ok := tlsVersionMap[t.MaxVersion]; ok {
			tlsConfig.MaxVersion = v
		}
	}

	// 如果配置了客户端证书，则加载 mTLS 配置
	clientCert := strings.TrimSpace(t.ClientCert)
	clientKey := strings.TrimSpace(t.ClientKey)
	caCert := strings.TrimSpace(t.CACert)

	if clientCert != "" && clientKey != "" {
		// 加载客户端证书和密钥
		cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// 加载 CA 证书
	if caCert != "" {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// tlsVersionMap TLS 版本映射
var tlsVersionMap = map[string]uint16{
	"1.0": tls.VersionTLS10,
	"1.1": tls.VersionTLS11,
	"1.2": tls.VersionTLS12,
	"1.3": tls.VersionTLS13,
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

	if ds.UpdatedAt == 0 {
		ds.UpdatedAt = time.Now().Unix()
	}
	return DB(ctx).Model(ds).Session(&gorm.Session{SkipHooks: true}).Select(selectField, selectFields...).Updates(ds).Error
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

type DatasourceInfo struct {
	Id         int64  `json:"id"`
	Name       string `json:"name"`
	PluginType string `json:"plugin_type"`
}

func GetDatasourceInfosByIds(ctx *ctx.Context, ids []int64) ([]*DatasourceInfo, error) {
	if len(ids) == 0 {
		return []*DatasourceInfo{}, nil
	}

	var dsInfos []*DatasourceInfo
	err := DB(ctx).
		Model(&Datasource{}).
		Select("id", "name", "plugin_type").
		Where("id in ?", ids).
		Find(&dsInfos).Error

	if err != nil {
		return nil, err
	}

	return dsInfos, nil
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
			if err := lst[i].Decrypt(); err != nil {
				logger.Errorf("decrypt datasource %+v fail: %v", lst[i], err)
				continue
			}
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

func GetDatasourcesCountByName(ctx *ctx.Context, name string) (int64, error) {
	session := DB(ctx).Model(&Datasource{})
	if name != "" {
		session = session.Where("name = ?", name)
	}

	return Count(session)
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

func GetDatasourcesGetsByTypes(ctx *ctx.Context, types []string) (map[string]*Datasource, error) {
	var lst []*Datasource
	m := make(map[string]*Datasource)
	err := DB(ctx).Where("plugin_type in ?", types).Find(&lst).Error
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

	if ds.PluginType == ELASTICSEARCH && len(ds.HTTPJson.Urls) == 0 {
		ds.HTTPJson.Urls = []string{ds.HTTPJson.Url}
	}

	if ds.Auth != "" {
		err := json.Unmarshal([]byte(ds.Auth), &ds.AuthJson)
		if err != nil {
			return err
		}
	}

	return nil
}

// Encrypt 数据源密码加密
func (ds *Datasource) Encrypt(openRsa bool, publicKeyData []byte) error {
	if !openRsa {
		return nil
	}

	if ds.Settings != "" {
		encVal, err := secu.EncryptValue(ds.Settings, publicKeyData)
		if err != nil {
			logger.Errorf("encrypt settings failed: datasource=%s err=%v", ds.Name, err)
			return err
		} else {
			ds.SettingsEncoded = encVal
		}
	}
	if ds.Auth != "" {
		encVal, err := secu.EncryptValue(ds.Auth, publicKeyData)
		if err != nil {
			logger.Errorf("encrypt basic failed: datasource=%s err=%v", ds.Name, err)
			return err
		} else {
			ds.AuthEncoded = encVal
		}
	}
	ds.ClearPlaintext()
	return nil
}

// Decrypt 用于 edge 将从中心同步的数据源解密，中心不可调用
func (ds *Datasource) Decrypt() error {
	if rsaConfig == nil {
		logger.Debugf("datasource %s rsa config is nil", ds.Name)
		return nil
	}

	if !rsaConfig.OpenRSA {
		return nil
	}

	privateKeyData := rsaConfig.PrivateKeyBytes
	password := rsaConfig.RSAPassWord
	if ds.SettingsEncoded != "" {
		settings, err := secu.Decrypt(ds.SettingsEncoded, privateKeyData, password)
		if err != nil {
			return err
		}
		ds.Settings = settings
		err = json.Unmarshal([]byte(settings), &ds.SettingsJson)
		if err != nil {
			return err
		}
	}

	if ds.AuthEncoded != "" {
		auth, err := secu.Decrypt(ds.AuthEncoded, privateKeyData, password)
		if err != nil {
			return err
		}
		ds.Auth = auth
		err = json.Unmarshal([]byte(auth), &ds.AuthJson)
		if err != nil {
			return err
		}
	}
	return nil
}

// ClearPlaintext 清理敏感字段
func (ds *Datasource) ClearPlaintext() {
	ds.Settings = ""
	ds.SettingsJson = nil

	ds.Auth = ""
	ds.AuthJson.BasicAuthUser = ""
	ds.AuthJson.BasicAuthPassword = ""
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
			if err := lst[i].Decrypt(); err != nil {
				logger.Errorf("decrypt datasource %+v fail: %v", lst[i], err)
				continue
			}
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

	ds := make(map[int64]*Datasource)
	for i := 0; i < len(lst); i++ {
		ds[lst[i].Id] = lst[i]
	}

	return ds, nil
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
