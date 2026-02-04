package router

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource/opensearch"
	"github.com/ccfos/nightingale/v6/dskit/clickhouse"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/logger"
)

func (rt *Router) pluginList(c *gin.Context) {
	Render(c, rt.Center.Plugins, nil)
}

type listReq struct {
	Name     string `json:"name"`
	Type     string `json:"plugin_type"`
	Category string `json:"category"`
}

func (rt *Router) datasourceList(c *gin.Context) {
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var req listReq
	ginx.BindJSON(c, &req)

	typ := req.Type
	category := req.Category
	name := req.Name

	user := c.MustGet("user").(*models.User)

	list, err := models.GetDatasourcesGetsBy(rt.Ctx, typ, category, name, "")
	Render(c, rt.DatasourceCache.DatasourceFilter(list, user), err)
}

func (rt *Router) datasourceGetsByService(c *gin.Context) {
	typ := ginx.QueryStr(c, "typ", "")
	lst, err := models.GetDatasourcesGetsBy(rt.Ctx, typ, "", "", "")

	openRsa := rt.Center.RSA.OpenRSA
	for _, item := range lst {
		if err := item.Encrypt(openRsa, rt.HTTP.RSA.RSAPublicKey); err != nil {
			logger.Errorf("datasource %+v encrypt failed: %v", item, err)
			continue
		}
	}
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) datasourceRsaConfigGet(c *gin.Context) {
	if rt.Center.RSA.OpenRSA {
		publicKey := ""
		privateKey := ""
		if len(rt.HTTP.RSA.RSAPublicKey) > 0 {
			publicKey = base64.StdEncoding.EncodeToString(rt.HTTP.RSA.RSAPublicKey)
		}
		if len(rt.HTTP.RSA.RSAPrivateKey) > 0 {
			privateKey = base64.StdEncoding.EncodeToString(rt.HTTP.RSA.RSAPrivateKey)
		}
		logger.Debugf("OpenRSA=%v", rt.Center.RSA.OpenRSA)
		ginx.NewRender(c).Data(models.RsaConfig{
			OpenRSA:       rt.Center.RSA.OpenRSA,
			RSAPublicKey:  publicKey,
			RSAPrivateKey: privateKey,
			RSAPassWord:   rt.HTTP.RSA.RSAPassWord,
		}, nil)
	} else {
		ginx.NewRender(c).Data(models.RsaConfig{
			OpenRSA: rt.Center.RSA.OpenRSA,
		}, nil)
	}
}

func (rt *Router) datasourceBriefs(c *gin.Context) {
	var dss []*models.Datasource
	list, err := models.GetDatasourcesGetsBy(rt.Ctx, "", "", "", "")
	ginx.Dangerous(err)

	for _, item := range list {
		item.AuthJson.BasicAuthPassword = ""
		if item.PluginType == models.PROMETHEUS {
			for k, v := range item.SettingsJson {
				if strings.HasPrefix(k, "prometheus.") {
					item.SettingsJson[strings.TrimPrefix(k, "prometheus.")] = v
					delete(item.SettingsJson, k)
				}
			}
		} else if item.PluginType == "cloudwatch" {
			for k := range item.SettingsJson {
				if !strings.Contains(k, "region") {
					delete(item.SettingsJson, k)
				}
			}
		} else {
			item.SettingsJson = nil
		}
		dss = append(dss, item)
	}

	if !rt.Center.AnonymousAccess.PromQuerier {
		user := c.MustGet("user").(*models.User)
		dss = rt.DatasourceCache.DatasourceFilter(dss, user)
	}

	ginx.NewRender(c).Data(dss, err)
}

func (rt *Router) datasourceUpsert(c *gin.Context) {
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var req models.Datasource
	ginx.BindJSON(c, &req)
	username := Username(c)
	req.UpdatedBy = username

	var err error
	var count int64

	if !req.ForceSave {
		if req.PluginType == models.PROMETHEUS || req.PluginType == models.LOKI || req.PluginType == models.TDENGINE {
			err = DatasourceCheck(c, req)
			if err != nil {
				Dangerous(c, err)
				return
			}
		}
	}

	for k, v := range req.SettingsJson {
		if strings.Contains(k, "cluster_name") {
			req.ClusterName = v.(string)
			break
		}
	}

	if req.PluginType == models.OPENSEARCH {
		b, err := json.Marshal(req.SettingsJson)
		if err != nil {
			logger.Warningf("marshal settings fail: %v", err)
			return
		}

		var os opensearch.OpenSearch
		err = json.Unmarshal(b, &os)
		if err != nil {
			logger.Warningf("unmarshal settings fail: %v", err)
			return
		}

		if len(os.Nodes) == 0 {
			logger.Warningf("nodes empty, %+v", req)
			return
		}

		req.HTTPJson = models.HTTP{
			Timeout: os.Timeout,
			Url:     os.Nodes[0],
			Headers: os.Headers,
			TLS: models.TLS{
				SkipTlsVerify: os.TLS.SkipTlsVerify,
			},
		}

		req.AuthJson = models.Auth{
			BasicAuth:         os.Basic.Enable,
			BasicAuthUser:     os.Basic.Username,
			BasicAuthPassword: os.Basic.Password,
		}
	}

	if req.PluginType == models.CLICKHOUSE {
		b, err := json.Marshal(req.SettingsJson)
		if err != nil {
			logger.Warningf("marshal clickhouse settings failed: %v", err)
			Dangerous(c, err)
			return
		}

		var ckConfig clickhouse.Clickhouse
		err = json.Unmarshal(b, &ckConfig)
		if err != nil {
			logger.Warningf("unmarshal clickhouse settings failed: %v", err)
			Dangerous(c, err)
			return
		}
		// 检查ckconfig的nodes不应该以http://或https://开头
		for _, addr := range ckConfig.Nodes {
			if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
				err = fmt.Errorf("clickhouse node address should not start with http:// or https:// : %s", addr)
				logger.Warningf("clickhouse node address invalid: %v", err)
				Dangerous(c, err)
				return
			}
		}

		// InitCli 会自动检测并选择 HTTP 或 Native 协议
		err = ckConfig.InitCli()
		if err != nil {
			logger.Warningf("clickhouse connection failed: %v", err)
			Dangerous(c, err)
			return
		}

		// 执行 SHOW DATABASES 测试连通性
		_, err = ckConfig.ShowDatabases(context.Background())
		if err != nil {
			logger.Warningf("clickhouse test query failed: %v", err)
			Dangerous(c, err)
			return
		}
	}

	if req.PluginType == models.ELASTICSEARCH {
		skipAuto := false
		// 若用户输入了version（version字符串存在且不为空），则不自动获取
		if req.SettingsJson != nil {
			if v, ok := req.SettingsJson["version"]; ok {
				switch vv := v.(type) {
				case string:
					if strings.TrimSpace(vv) != "" {
						skipAuto = true
					}
				default:
					if strings.TrimSpace(fmt.Sprint(vv)) != "" {
						skipAuto = true
					}
				}
			}
		}

		if !skipAuto {
			version, err := getElasticsearchVersion(req, 10*time.Second)
			if err != nil {
				logger.Warningf("failed to get elasticsearch version: %v", err)
			} else {
				if req.SettingsJson == nil {
					req.SettingsJson = make(map[string]interface{})
				}
				req.SettingsJson["version"] = version
			}
		}
	}

	if req.Id == 0 {
		req.CreatedBy = username
		req.Status = "enabled"
		count, err = models.GetDatasourcesCountBy(rt.Ctx, "", "", req.Name)
		if err != nil {
			Render(c, nil, err)
			return
		}

		if count > 0 {
			Render(c, nil, "name already exists")
			return
		}
		err = req.Add(rt.Ctx)
	} else {
		err = req.Update(rt.Ctx, "name", "identifier", "description", "cluster_name", "settings", "http", "auth", "updated_by", "updated_at", "is_default", "weight")
	}

	Render(c, nil, err)
}

func DatasourceCheck(c *gin.Context, ds models.Datasource) error {
	if ds.PluginType == models.PROMETHEUS || ds.PluginType == models.LOKI || ds.PluginType == models.TDENGINE {
		if ds.HTTPJson.Url == "" {
			return fmt.Errorf("url is empty")
		}

		if !strings.HasPrefix(ds.HTTPJson.Url, "http") {
			return fmt.Errorf("url must start with http or https")
		}
	}

	// 使用 TLS 配置（支持 mTLS）
	tlsConfig, err := ds.HTTPJson.TLS.TLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create TLS config: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	ds.HTTPJson.Url = strings.TrimRight(ds.HTTPJson.Url, "/")
	var fullURL string
	req, err := ds.HTTPJson.NewReq(&fullURL)
	if err != nil {
		logger.Errorf("Error creating request: %v", err)
		return fmt.Errorf("request urls:%v failed: %v", ds.HTTPJson.GetUrls(), err)
	}

	if ds.PluginType == models.PROMETHEUS {
		subPath := "/api/v1/query"
		query := url.Values{}
		if ds.HTTPJson.IsLoki() {
			subPath = "/api/v1/labels"
		} else {
			query.Add("query", "1+1")
		}
		fullURL = fmt.Sprintf("%s%s?%s", ds.HTTPJson.Url, subPath, query.Encode())

		req, err = http.NewRequest("GET", fullURL, nil)
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			return fmt.Errorf("request url:%s failed: %v", fullURL, err)
		}
	} else if ds.PluginType == models.TDENGINE {
		fullURL = fmt.Sprintf("%s/rest/sql", ds.HTTPJson.Url)
		req, err = http.NewRequest("POST", fullURL, strings.NewReader("show databases"))
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			return fmt.Errorf("request url:%s failed: %v", fullURL, err)
		}
	}

	if ds.PluginType == models.LOKI {
		subPath := "/api/v1/labels"

		fullURL = fmt.Sprintf("%s%s", ds.HTTPJson.Url, subPath)

		req, err = http.NewRequest("GET", fullURL, nil)
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			if !strings.Contains(ds.HTTPJson.Url, "/loki") {
				lang := c.GetHeader("X-Language")
				return fmt.Errorf(i18n.Sprintf(lang, "/loki suffix is miss, please add /loki to the url: %s", ds.HTTPJson.Url+"/loki"))
			}
			return fmt.Errorf("request url:%s failed: %v", fullURL, err)
		}
	}

	if ds.AuthJson.BasicAuthUser != "" {
		req.SetBasicAuth(ds.AuthJson.BasicAuthUser, ds.AuthJson.BasicAuthPassword)
	}

	for k, v := range ds.HTTPJson.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("Error making request: %v\n", err)
		return fmt.Errorf("request url:%s failed: %v", fullURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Errorf("Error making request: %v\n", resp.StatusCode)
		if resp.StatusCode == 404 && ds.PluginType == models.LOKI && !strings.Contains(ds.HTTPJson.Url, "/loki") {
			lang := c.GetHeader("X-Language")
			return fmt.Errorf(i18n.Sprintf(lang, "/loki suffix is miss, please add /loki to the url: %s", ds.HTTPJson.Url+"/loki"))
		}
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request url:%s failed code:%d body:%s", fullURL, resp.StatusCode, string(body))
	}

	return nil
}

func (rt *Router) datasourceGet(c *gin.Context) {
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var req models.Datasource
	ginx.BindJSON(c, &req)
	err := req.Get(rt.Ctx)
	Render(c, req, err)
}

func (rt *Router) datasourceUpdataStatus(c *gin.Context) {
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var req models.Datasource
	ginx.BindJSON(c, &req)
	username := Username(c)
	req.UpdatedBy = username
	err := req.Update(rt.Ctx, "status", "updated_by", "updated_at")
	Render(c, req, err)
}

func (rt *Router) datasourceDel(c *gin.Context) {
	if rt.DatasourceCache.DatasourceCheckHook(c) {
		Render(c, []int{}, nil)
		return
	}

	var ids []int64
	ginx.BindJSON(c, &ids)
	err := models.DatasourceDel(rt.Ctx, ids)
	Render(c, nil, err)
}

func (rt *Router) getDatasourceIds(c *gin.Context) {
	name := ginx.QueryStr(c, "name")
	datasourceIds, err := models.GetDatasourceIdsByEngineName(rt.Ctx, name)

	ginx.NewRender(c).Data(datasourceIds, err)
}

type datasourceQueryForm struct {
	Cate              string                   `json:"datasource_cate"`
	DatasourceQueries []models.DatasourceQuery `json:"datasource_queries"`
}

type datasourceQueryResp struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (rt *Router) datasourceQuery(c *gin.Context) {
	var dsf datasourceQueryForm
	ginx.BindJSON(c, &dsf)
	datasources, err := models.GetDatasourcesGetsByTypes(rt.Ctx, []string{dsf.Cate})
	ginx.Dangerous(err)

	nameToID := make(map[string]int64)
	IDToName := make(map[int64]string)
	for _, ds := range datasources {
		nameToID[ds.Name] = ds.Id
		IDToName[ds.Id] = ds.Name
	}

	ids := models.GetDatasourceIDsByDatasourceQueries(dsf.DatasourceQueries, IDToName, nameToID)
	var req []datasourceQueryResp
	for _, id := range ids {
		req = append(req, datasourceQueryResp{
			ID:   id,
			Name: IDToName[id],
		})
	}
	ginx.NewRender(c).Data(req, err)
}

// getElasticsearchVersion 该函数尝试从提供的Elasticsearch数据源中获取版本号，遍历所有URL，
// 直到成功获取版本号或所有URL均尝试失败为止。
func getElasticsearchVersion(ds models.Datasource, timeout time.Duration) (string, error) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: ds.HTTPJson.TLS.SkipTlsVerify,
			},
		},
	}

	urls := make([]string, 0)
	if len(ds.HTTPJson.Urls) > 0 {
		urls = append(urls, ds.HTTPJson.Urls...)
	}
	if ds.HTTPJson.Url != "" {
		urls = append(urls, ds.HTTPJson.Url)
	}
	if len(urls) == 0 {
		return "", fmt.Errorf("no url provided")
	}

	var lastErr error
	for _, raw := range urls {
		baseURL := strings.TrimRight(raw, "/") + "/"
		req, err := http.NewRequest("GET", baseURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		if ds.AuthJson.BasicAuthUser != "" {
			req.SetBasicAuth(ds.AuthJson.BasicAuthUser, ds.AuthJson.BasicAuthPassword)
		}

		for k, v := range ds.HTTPJson.Headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("request to %s failed with status: %d body:%s", baseURL, resp.StatusCode, string(body))
			continue
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = err
			continue
		}

		if version, ok := result["version"].(map[string]interface{}); ok {
			if number, ok := version["number"].(string); ok && number != "" {
				return number, nil
			}
		}

		lastErr = fmt.Errorf("version not found in response from %s", baseURL)
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("failed to get elasticsearch version")
}
