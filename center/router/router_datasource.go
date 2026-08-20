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
	"regexp"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource/opensearch"
	"github.com/ccfos/nightingale/v6/dskit/clickhouse"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
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
	list = rt.DatasourceCache.DatasourceFilter(list, user)

	if !user.IsAdmin() {
		for _, ds := range list {
			ds.RedactSecrets()
		}
	}

	Render(c, list, err)
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
		// 先挑出 UI 必需且不敏感的 settings 字段，再统一调用 RedactSecrets
		// 全量脱敏，避免遗漏 HTTPJson.Headers / AuthEncoded / SettingsEncoded
		// 之类同样会泄露密钥的字段。
		var safeSettings map[string]interface{}
		switch item.PluginType {
		case models.PROMETHEUS:
			safeSettings = make(map[string]interface{})
			for k, v := range item.SettingsJson {
				switch {
				case strings.HasPrefix(k, "prometheus."):
					safeSettings[strings.TrimPrefix(k, "prometheus.")] = v
				case k == "write_addr", k == "internal_addr":
					safeSettings[k] = v
				}
			}
		case "cloudwatch":
			safeSettings = make(map[string]interface{})
			for k, v := range item.SettingsJson {
				if strings.Contains(k, "region") {
					safeSettings[k] = v
				}
			}
		}

		item.RedactSecrets()
		item.SettingsJson = safeSettings
		dss = append(dss, item)
	}

	if bid, ok := boardTokenBid(c); ok {
		// 分享 token 请求：只暴露板内引用的数据源，不泄露全站清单；且抹掉连接地址等
		// 网络拓扑信息（RedactSecrets 不清 HTTPJson.Url/Urls，上面又把 write_addr/
		// internal_addr/prometheus.* 填回 SettingsJson），匿名调用方只需 id/name/
		// plugin_type/identifier/cluster_name 把 datasourceValue 映射成名字。
		// 这些 item 来自本次 GetDatasourcesGetsBy 查询结果，非缓存共享指针，就地改写安全。
		set, e := rt.boardDsSet(bid)
		ginx.Dangerous(e)
		filtered := make([]*models.Datasource, 0, len(set))
		for _, item := range dss {
			if _, has := set[item.Id]; has {
				item.HTTPJson = models.HTTP{}
				item.SettingsJson = nil
				filtered = append(filtered, item)
			}
		}
		dss = filtered
	} else if !rt.Center.AnonymousAccess.PromQuerier {
		user := c.MustGet("user").(*models.User)
		dss = rt.DatasourceCache.DatasourceFilter(dss, user)
	}

	ginx.NewRender(c).Data(dss, err)
}

// datasourceVerification 是 upsert 保存时连通性/查询测试的结构化结果，
// 用于前端保存结果页区分「已验证 / 已保存未验证」。
//
// 当前实现只会产出前两种状态：测试没过一律阻断保存（走 Dangerous），因此不存在
// 「存下来了但没通过」的情况；force_save 则一次测试都不发，保持 saved_unverified。
// force_saved_failed 仅保留在契约里 —— 前端对它有兜底渲染，将来若把连通性测试
// 改成保存后异步执行，这个状态会重新有来源。
type datasourceVerification struct {
	State     string `json:"state"` // verified | saved_unverified（force_saved_failed 暂无来源）
	Stage     string `json:"stage"` // query | version | none
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message"` // 脱敏后的错误信息；目前仅 elasticsearch 取版本失败时透出
}

// 错误信息中可能出现用户写进 URL 的凭据（http://user:pass@host），透出前必须脱敏
var dsUrlCredRe = regexp.MustCompile(`(://)[^/@\s]+:[^/@\s]+@`)

func sanitizeDsError(msg string) string {
	return dsUrlCredRe.ReplaceAllString(msg, "$1***:***@")
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

	verif := datasourceVerification{State: "saved_unverified", Stage: "none"}

	// runCheck 执行一次测试并记录结构化结果。
	// 返回 true 表示已响应错误（失败且未选择强制保存），调用方需直接 return。
	runCheck := func(stage string, fn func() error) (aborted bool) {
		// force_save 对应前端「不测试连通性直接保存」：一次测试都不发，保存立即返回，
		// verif 保持 saved_unverified/none。连通性交给保存结果弹窗的数据体检去回答 ——
		// 若在这里跑测试，不可达地址会让保存请求挂满超时，与按钮承诺的「不测试」不符。
		// 注意 clickhouse 也走本函数，因此它同样受此门禁保护（旧实现里 ck 的连通性测试
		// 未受 force_save 保护，导致「直接保存」在 ck 上必失败）。
		if req.ForceSave {
			return false
		}

		t0 := time.Now()
		checkErr := fn()
		verif.Stage = stage
		verif.LatencyMs = time.Since(t0).Milliseconds()
		if checkErr == nil {
			verif.State = "verified"
			return false
		}
		// 走到这里必然是「测试连通性并保存」且没通过：如实报错、不落库。
		// 这条错误会原样回给前端展示，因此同样要过脱敏 —— 用户可能把凭据写在
		// URL 里（http://user:pass@host），而 http 库的报错会带上整个 URL。
		Dangerous(c, fmt.Errorf("%s", sanitizeDsError(checkErr.Error())))
		return true
	}

	if req.PluginType == models.PROMETHEUS || req.PluginType == models.LOKI || req.PluginType == models.TDENGINE || req.PluginType == models.IOTDB {
		if runCheck("query", func() error { return DatasourceCheck(c, req) }) {
			return
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
		// 配置格式错误始终阻断（不属于连通性问题，force_save 也不应保存脏配置）
		for _, addr := range ckConfig.Nodes {
			if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
				err = fmt.Errorf("clickhouse node address should not start with http:// or https:// : %s", addr)
				logger.Warningf("clickhouse node address invalid: %v", err)
				Dangerous(c, err)
				return
			}
		}

		// 连通性测试统一走 runCheck，从而受 force_save 门禁保护
		//（旧实现这段没有门禁，导致「直接保存」在 ck 上必失败）
		if runCheck("query", func() error {
			// InitCli 会自动检测并选择 HTTP 或 Native 协议
			if initErr := ckConfig.InitCli(); initErr != nil {
				logger.Warningf("clickhouse connection failed: %v", initErr)
				return initErr
			}
			// 执行 SHOW DATABASES 测试连通性
			if _, qErr := ckConfig.ShowDatabases(context.Background()); qErr != nil {
				logger.Warningf("clickhouse test query failed: %v", qErr)
				return qErr
			}
			return nil
		}) {
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
			t0 := time.Now()
			version, verErr := getElasticsearchVersion(req, 10*time.Second)
			verif.Stage = "version"
			verif.LatencyMs = time.Since(t0).Milliseconds()
			if verErr != nil {
				// 保持既有语义：取版本失败不阻断保存，但如实记录「未验证」及原因
				logger.Warningf("failed to get elasticsearch version: %v", verErr)
				verif.Message = sanitizeDsError(verErr.Error())
			} else {
				verif.State = "verified"
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

	if err != nil {
		Render(c, nil, err)
		return
	}

	// 主动刷新数据源缓存：保存结果页会立刻发起数据体检、用户也可能马上去探索器查询，
	// 若等 9 秒的下一个同步周期，proxy 会因缓存里没有这条记录而报 "no such datasource"。
	if syncErr := rt.DatasourceCache.SyncOnce(); syncErr != nil {
		logger.Warningf("sync datasource cache after upsert failed: %v", syncErr)
	}

	// req.Add 经 gorm Create 回填自增 Id；前端保存结果页依赖 id 与 verification 建立上下文
	Render(c, gin.H{
		"id":           req.Id,
		"verification": verif,
	}, nil)
}

func DatasourceCheck(c *gin.Context, ds models.Datasource) error {
	if ds.PluginType == models.PROMETHEUS || ds.PluginType == models.LOKI || ds.PluginType == models.TDENGINE || ds.PluginType == models.IOTDB {
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
		// 必须有超时：不可达地址会一直挂住「测试连通性并保存」这个请求
		Timeout: 10 * time.Second,
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
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else if ds.PluginType == models.IOTDB {
		fullURL = fmt.Sprintf("%s/rest/table/v1/query", ds.HTTPJson.Url)
		req, err = http.NewRequest("POST", fullURL, strings.NewReader(`{"sql":"show databases"}`))
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			return fmt.Errorf("request url:%s failed: %v", fullURL, err)
		}
		req.Header.Set("Content-Type", "application/json")
	}

	if ds.PluginType == models.LOKI {
		subPath := "/api/v1/labels"

		fullURL = fmt.Sprintf("%s%s", ds.HTTPJson.Url, subPath)

		req, err = http.NewRequest("GET", fullURL, nil)
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			if !strings.Contains(ds.HTTPJson.Url, "/loki") {
				lang := c.GetHeader("X-Language")
				return newMessageError(i18n.Sprintf(lang, "/loki suffix is miss, please add /loki to the url: %s", ds.HTTPJson.Url+"/loki"))
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
			return newMessageError(i18n.Sprintf(lang, "/loki suffix is miss, please add /loki to the url: %s", ds.HTTPJson.Url+"/loki"))
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
