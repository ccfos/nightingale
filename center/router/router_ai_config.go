package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

// ========================
// LLM Provider handlers
// ========================

func (rt *Router) llmProviderGets(c *gin.Context) {
	lst, err := models.LLMProviderGets(rt.Ctx)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) llmProviderAdd(c *gin.Context) {
	var obj models.LLMProvider
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) llmProviderPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.LLMProviderGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "llm provider not found")
	}

	var ref models.LLMProvider
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) llmProviderDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.LLMProviderGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "llm provider not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// ========================
// AI Skill handlers
// ========================

func (rt *Router) aiSkillGets(c *gin.Context) {
	search := ginx.QueryStr(c, "search", "")
	lst, err := models.AISkillGets(rt.Ctx, search)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) aiSkillGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillAdd(c *gin.Context) {
	var obj models.AISkill
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiSkillPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	var ref models.AISkill
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) aiSkillDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	if obj.IsBuiltin == 1 {
		ginx.Bomb(http.StatusForbidden, "builtin skill cannot be deleted")
	}

	// Cascade delete skill files
	ginx.Dangerous(models.AISkillFileDeleteBySkillId(rt.Ctx, id))
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) aiSkillImport(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	ginx.Dangerous(err)
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".md" {
		ginx.Bomb(http.StatusBadRequest, "only .md files are supported")
	}

	content, err := io.ReadAll(file)
	ginx.Dangerous(err)

	name := strings.TrimSuffix(header.Filename, ext)
	me := c.MustGet("user").(*models.User)

	skill := models.AISkill{
		Name:         name,
		Instructions: string(content),
		CreatedBy:    me.Username,
		UpdatedBy:    me.Username,
	}
	ginx.Dangerous(skill.Create(rt.Ctx))
	ginx.NewRender(c).Data(skill.Id, nil)
}

// ========================
// AI Skill File handlers
// ========================

func (rt *Router) aiSkillFileGets(c *gin.Context) {
	skillId := ginx.UrlParamInt64(c, "id")
	lst, err := models.AISkillFileGets(rt.Ctx, skillId)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) aiSkillFileAdd(c *gin.Context) {
	skillId := ginx.UrlParamInt64(c, "id")

	// Verify skill exists
	skill, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if skill == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	file, header, err := c.Request.FormFile("file")
	ginx.Dangerous(err)
	defer file.Close()

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".md": true, ".txt": true, ".json": true, ".yaml": true, ".yml": true, ".csv": true}
	if !allowed[ext] {
		ginx.Bomb(http.StatusBadRequest, "file type not allowed, only .md/.txt/.json/.yaml/.csv")
	}

	// Validate file size (2MB max)
	if header.Size > 2*1024*1024 {
		ginx.Bomb(http.StatusBadRequest, "file size exceeds 2MB limit")
	}

	content, err := io.ReadAll(file)
	ginx.Dangerous(err)

	me := c.MustGet("user").(*models.User)
	skillFile := models.AISkillFile{
		SkillId:   skillId,
		Name:      header.Filename,
		Content:   string(content),
		CreatedBy: me.Username,
	}
	ginx.Dangerous(skillFile.Create(rt.Ctx))
	ginx.NewRender(c).Data(skillFile.Id, nil)
}

func (rt *Router) aiSkillFileGet(c *gin.Context) {
	fileId := ginx.UrlParamInt64(c, "fileId")
	obj, err := models.AISkillFileGetById(rt.Ctx, fileId)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "file not found")
	}
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillFileDel(c *gin.Context) {
	fileId := ginx.UrlParamInt64(c, "fileId")
	obj, err := models.AISkillFileGetById(rt.Ctx, fileId)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "file not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// ========================
// MCP Server handlers
// ========================

func (rt *Router) mcpServerGets(c *gin.Context) {
	lst, err := models.MCPServerGets(rt.Ctx)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) mcpServerAdd(c *gin.Context) {
	var obj models.MCPServer
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) mcpServerPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	var ref models.MCPServer
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	ref.UpdatedBy = me.Username

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, ref))
}

func (rt *Router) mcpServerDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// ========================
// LLM Provider test
// ========================

func (rt *Router) llmProviderTest(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	// Try to read override values from request body
	var body struct {
		APIType string `json:"api_type"`
		APIURL  string `json:"api_url"`
		APIKey  string `json:"api_key"`
		Model   string `json:"model"`
	}
	c.ShouldBindJSON(&body)

	var obj *models.LLMProvider

	if id > 0 {
		// Load from database as base
		var err error
		obj, err = models.LLMProviderGetById(rt.Ctx, id)
		ginx.Dangerous(err)
		if obj == nil {
			ginx.Bomb(http.StatusNotFound, "llm provider not found")
		}
		// Override with body values if provided
		if body.APIType != "" {
			obj.APIType = body.APIType
		}
		if body.APIURL != "" {
			obj.APIURL = body.APIURL
		}
		if body.APIKey != "" {
			obj.APIKey = body.APIKey
		}
		if body.Model != "" {
			obj.Model = body.Model
		}
	} else {
		// No id, use body values directly (new provider test)
		if body.APIType == "" || body.APIURL == "" || body.APIKey == "" || body.Model == "" {
			ginx.Bomb(http.StatusBadRequest, "api_type, api_url, api_key, model are required")
		}
		obj = &models.LLMProvider{
			APIType: body.APIType,
			APIURL:  body.APIURL,
			APIKey:  body.APIKey,
			Model:   body.Model,
		}
	}

	start := time.Now()
	testErr := testLLMProvider(obj)
	durationMs := time.Since(start).Milliseconds()

	result := gin.H{
		"success":     testErr == nil,
		"duration_ms": durationMs,
	}
	if testErr != nil {
		result["error"] = testErr.Error()
	}
	ginx.NewRender(c).Data(result, nil)
}

func testLLMProvider(p *models.LLMProvider) error {
	client := &http.Client{Timeout: 30 * time.Second}

	var reqURL string
	var reqBody []byte
	hdrs := map[string]string{"Content-Type": "application/json"}

	switch p.APIType {
	case "openai":
		base := strings.TrimRight(p.APIURL, "/")
		if strings.HasSuffix(base, "/chat/completions") {
			reqURL = base
		} else {
			reqURL = base + "/chat/completions"
		}
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
			"max_tokens": 5,
		})
		hdrs["Authorization"] = "Bearer " + p.APIKey
	case "claude":
		reqURL = strings.TrimRight(p.APIURL, "/") + "/v1/messages"
		reqBody, _ = json.Marshal(map[string]interface{}{
			"model":      p.Model,
			"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
			"max_tokens": 5,
		})
		hdrs["x-api-key"] = p.APIKey
		hdrs["anthropic-version"] = "2023-06-01"
	case "gemini":
		reqURL = strings.TrimRight(p.APIURL, "/") + "/v1beta/models/" + p.Model + ":generateContent?key=" + p.APIKey
		reqBody, _ = json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]string{{"text": "Hi"}}},
			},
		})
	default:
		return fmt.Errorf("unsupported api_type: %s", p.APIType)
	}

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 500 {
			body = body[:500]
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ========================
// MCP Server test & tools
// ========================

func (rt *Router) mcpServerTest(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	start := time.Now()
	tools, testErr := listMCPTools(obj)
	durationMs := time.Since(start).Milliseconds()

	result := gin.H{
		"success":     testErr == nil,
		"duration_ms": durationMs,
		"tool_count":  len(tools),
	}
	if testErr != nil {
		result["error"] = testErr.Error()
	}
	ginx.NewRender(c).Data(result, nil)
}

func (rt *Router) mcpServerTools(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.MCPServerGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "mcp server not found")
	}

	tools, err := listMCPTools(obj)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(tools, nil)
}

type mcpTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func listMCPTools(s *models.MCPServer) ([]mcpTool, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	hdrs := make(map[string]string)
	if s.Headers != "" {
		json.Unmarshal([]byte(s.Headers), &hdrs)
	}

	// Step 1: Initialize
	initResp, initSessionID, err := sendMCPRPC(client, s.URL, hdrs, "", 1, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "nightingale", "version": "1.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize: %v", err)
	}
	_ = initResp

	// Send initialized notification
	sendMCPRPC(client, s.URL, hdrs, initSessionID, 0, "notifications/initialized", map[string]interface{}{})

	// Step 2: List tools
	toolsResp, _, err := sendMCPRPC(client, s.URL, hdrs, initSessionID, 2, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %v", err)
	}

	if toolsResp == nil || toolsResp.Result == nil {
		return []mcpTool{}, nil
	}

	toolsRaw, ok := toolsResp.Result["tools"]
	if !ok {
		return []mcpTool{}, nil
	}

	toolsJSON, _ := json.Marshal(toolsRaw)
	var tools []mcpTool
	json.Unmarshal(toolsJSON, &tools)
	return tools, nil
}

type jsonRPCResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result"`
	Error   *jsonRPCError          `json:"error"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func sendMCPRPC(client *http.Client, serverURL string, hdrs map[string]string, sessionID string, id int, method string, params interface{}) (*jsonRPCResponse, string, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	if id > 0 {
		body["id"] = id
	}

	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", serverURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	newSessionID := resp.Header.Get("Mcp-Session-Id")
	if newSessionID == "" {
		newSessionID = sessionID
	}

	// Notification (no id) - no response body expected
	if id <= 0 {
		return nil, newSessionID, nil
	}

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		if len(respBody) > 500 {
			respBody = respBody[:500]
		}
		return nil, newSessionID, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newSessionID, err
	}

	// Handle SSE response
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		for _, line := range strings.Split(string(respBody), "\n") {
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var rpcResp jsonRPCResponse
				if json.Unmarshal([]byte(data), &rpcResp) == nil && (rpcResp.Result != nil || rpcResp.Error != nil) {
					if rpcResp.Error != nil {
						return &rpcResp, newSessionID, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
					}
					return &rpcResp, newSessionID, nil
				}
			}
		}
		return nil, newSessionID, fmt.Errorf("no valid JSON-RPC response in SSE stream")
	}

	// Handle JSON response
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		if len(respBody) > 200 {
			respBody = respBody[:200]
		}
		return nil, newSessionID, fmt.Errorf("invalid response: %s", string(respBody))
	}

	if rpcResp.Error != nil {
		return &rpcResp, newSessionID, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, newSessionID, nil
}
