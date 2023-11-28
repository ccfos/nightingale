package router

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
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
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) datasourceBriefs(c *gin.Context) {
	var dss []*models.Datasource
	list, err := models.GetDatasourcesGetsBy(rt.Ctx, "", "", "", "")
	ginx.Dangerous(err)

	for i := range list {
		dss = append(dss, &models.Datasource{
			Id:         list[i].Id,
			Name:       list[i].Name,
			PluginType: list[i].PluginType,
		})
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

	err = DatasourceCheck(req)
	if err != nil {
		Dangerous(c, err)
		return
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
		err = req.Update(rt.Ctx, "name", "description", "cluster_name", "settings", "http", "auth", "updated_by", "updated_at", "is_default")
	}

	Render(c, nil, err)
}

func DatasourceCheck(ds models.Datasource) error {
	if ds.HTTPJson.Url == "" {
		return fmt.Errorf("url is empty")
	}

	if !strings.HasPrefix(ds.HTTPJson.Url, "http") {
		return fmt.Errorf("url must start with http or https")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: ds.HTTPJson.TLS.SkipTlsVerify,
			},
		},
	}

	fullURL := ds.HTTPJson.Url
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		logger.Errorf("Error creating request: %v", err)
		return fmt.Errorf("request url:%s failed", fullURL)
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
			return fmt.Errorf("request url:%s failed", fullURL)
		}
	} else if ds.PluginType == models.TDENGINE {
		fullURL = fmt.Sprintf("%s/rest/sql", ds.HTTPJson.Url)
		req, err = http.NewRequest("POST", fullURL, strings.NewReader("show databases"))
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			return fmt.Errorf("request url:%s failed", fullURL)
		}
	}

	if ds.PluginType == models.LOKI {
		subPath := "/api/v1/labels"

		fullURL = fmt.Sprintf("%s%s", ds.HTTPJson.Url, subPath)

		req, err = http.NewRequest("GET", fullURL, nil)
		if err != nil {
			logger.Errorf("Error creating request: %v", err)
			return fmt.Errorf("request url:%s failed", fullURL)
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
		return fmt.Errorf("request url:%s failed", fullURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logger.Errorf("Error making request: %v\n", resp.StatusCode)
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

func Username(c *gin.Context) string {

	return c.MustGet("username").(string)
}
