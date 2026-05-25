package router

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llmconfig"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/i18nx"

	"github.com/gin-gonic/gin"
)

func isV1Request(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/v1")
}

func (rt *Router) aiLLMConfigGets(c *gin.Context) {
	lst, err := models.AILLMConfigGets(rt.Ctx)
	if !isV1Request(c) {
		for _, item := range lst {
			item.MaskAPIKey()
		}
	}
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) aiLLMConfigGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}
	if !isV1Request(c) {
		obj.MaskAPIKey()
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiLLMConfigAdd(c *gin.Context) {
	var obj models.AILLMConfig
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.Dangerous(obj.Create(rt.Ctx, me.Username))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiLLMConfigPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}

	var ref models.AILLMConfig
	ginx.BindJSON(c, &ref)
	// Treat empty or the masked round-trip value as "keep existing key".
	// GET returns api_key masked (e.g. "sk-a****wxyz"); if the frontend PUTs
	// that mask back unchanged we must not overwrite the real key with it.
	if ref.APIKey == "" || models.IsMaskedAPIKey(ref.APIKey, obj.APIKey) {
		ref.APIKey = obj.APIKey
	}
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, me.Username, ref))
}

func (rt *Router) aiLLMConfigAddByService(c *gin.Context) {
	var obj models.AILLMConfig
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	ginx.Dangerous(obj.Create(rt.Ctx, "system"))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiLLMConfigPutByService(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}

	var ref models.AILLMConfig
	ginx.BindJSON(c, &ref)
	if ref.APIKey == "" || models.IsMaskedAPIKey(ref.APIKey, obj.APIKey) {
		ref.APIKey = obj.APIKey
	}
	ginx.Dangerous(ref.Verify())

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, "system", ref))
}

func (rt *Router) aiLLMConfigDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AILLMConfigGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai llm config not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) aiLLMConfigTest(c *gin.Context) {
	var body struct {
		Name        string                `json:"name"`
		APIType     string                `json:"api_type"`
		APIURL      string                `json:"api_url"`
		APIKey      string                `json:"api_key"`
		Model       string                `json:"model"`
		ExtraConfig models.LLMExtraConfig `json:"extra_config"`
	}
	ginx.BindJSON(c, &body)

	if body.APIType == "" || body.APIURL == "" || body.APIKey == "" || body.Model == "" {
		ginx.Bomb(http.StatusBadRequest, "api_type, api_url, api_key, model are required")
	}

	// On the edit page the GET response masks api_key (e.g. "sk-a****wxyz").
	// If the frontend posts that masked value back for testing, look up the
	// real key by name so the test actually authenticates.
	if body.Name != "" {
		stored, err := models.AILLMConfigGetByName(rt.Ctx, body.Name)
		ginx.Dangerous(err)
		if stored != nil && models.IsMaskedAPIKey(body.APIKey, stored.APIKey) {
			body.APIKey = stored.APIKey
		}
	}

	obj := &models.AILLMConfig{
		APIType:     body.APIType,
		APIURL:      body.APIURL,
		APIKey:      body.APIKey,
		Model:       body.Model,
		ExtraConfig: body.ExtraConfig,
	}

	start := time.Now()
	testErr := llmconfig.Test(obj)
	durationMs := time.Since(start).Milliseconds()

	result := gin.H{
		"success":     testErr == nil,
		"duration_ms": durationMs,
	}
	if testErr != nil {
		var errMsg string
		var probeErr *llmconfig.ProbeError
		if errors.As(testErr, &probeErr) {
			errMsg = translateProbeError(c.GetHeader("X-Language"), probeErr)
		} else {
			errMsg = testErr.Error()
		}
		result["error"] = errMsg
	}
	ginx.NewRender(c).Data(result, nil)
}

func translateProbeError(lang string, err *llmconfig.ProbeError) string {
	switch err.Kind {
	case llmconfig.ProbeErrorAuth:
		return i18nx.Translatef(lang, "llm test: authentication failed (HTTP %d), please verify your API Key is correct. server response: %s", err.StatusCode, err.Detail)
	case llmconfig.ProbeErrorEndpointNotFound:
		return i18nx.Translatef(lang, "llm test: endpoint not found (HTTP 404), please check your API URL (current: %s). for OpenAI-compatible APIs the URL should end with /v1, e.g. https://api.openai.com/v1. server response: %s", err.APIURL, err.Detail)
	case llmconfig.ProbeErrorRateLimited:
		return i18nx.Translatef(lang, "llm test: rate limit exceeded (HTTP 429), your API Key quota is exhausted or requests are too frequent. server response: %s", err.Detail)
	case llmconfig.ProbeErrorRequestFailed:
		return i18nx.Translatef(lang, "llm test: request failed (HTTP %d). server response: %s", err.StatusCode, err.Detail)
	case llmconfig.ProbeErrorUnexpectedResponse:
		return i18nx.Translatef(lang, "llm test: unexpected response format, the API URL may point to a wrong endpoint. raw: %s", err.Detail)
	case llmconfig.ProbeErrorModel:
		return i18nx.Translatef(lang, "llm test: LLM returned an error for model(%s): %s, please verify the model name is correct", err.Model, err.Detail)
	case llmconfig.ProbeErrorNoContent:
		return i18nx.Translatef(lang, "llm test: LLM returned no content, please verify the model name(%s) is correct", err.Model)
	default:
		return err.Error()
	}
}
