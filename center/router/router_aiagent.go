package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// AIChatRequest is the generic chat request dispatched by action_key.
type AIChatRequest struct {
	ActionKey string                `json:"action_key"` // e.g. "query_generator"
	UserInput string                `json:"user_input"`
	History   []aiagent.ChatMessage `json:"history,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"` // action-specific params
}

// actionHandler defines how each action_key is processed.
type actionHandler struct {
	useCase     string // maps to AIAgent.UseCase for finding the right agent config
	validate    func(req *AIChatRequest) error
	selectTools func(req *AIChatRequest) []string
	buildPrompt func(req *AIChatRequest) string
	buildInputs func(req *AIChatRequest) map[string]string
}

var actionRegistry = map[string]*actionHandler{
	"query_generator": {
		useCase:     "chat",
		validate:    validateQueryGenerator,
		selectTools: selectQueryGeneratorTools,
		buildPrompt: buildQueryGeneratorPrompt,
		buildInputs: buildQueryGeneratorInputs,
	},
}

// --- query_generator action ---

func ctxStr(ctx map[string]interface{}, key string) string {
	if v, ok := ctx[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func ctxInt64(ctx map[string]interface{}, key string) int64 {
	if v, ok := ctx[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int64:
			return n
		case json.Number:
			i, _ := n.Int64()
			return i
		}
	}
	return 0
}

func validateQueryGenerator(req *AIChatRequest) error {
	dsType := ctxStr(req.Context, "datasource_type")
	dsID := ctxInt64(req.Context, "datasource_id")
	if dsType == "" {
		return fmt.Errorf("context.datasource_type is required")
	}
	if dsID == 0 {
		return fmt.Errorf("context.datasource_id is required")
	}
	return nil
}

func selectQueryGeneratorTools(req *AIChatRequest) []string {
	dsType := ctxStr(req.Context, "datasource_type")
	switch dsType {
	case "prometheus":
		return []string{"list_metrics", "get_metric_labels"}
	case "mysql", "doris", "ck", "clickhouse", "pgsql", "postgresql":
		return []string{"list_databases", "list_tables", "describe_table"}
	default:
		return nil
	}
}

func buildQueryGeneratorPrompt(req *AIChatRequest) string {
	dsType := ctxStr(req.Context, "datasource_type")
	dbName := ctxStr(req.Context, "database_name")
	tableName := ctxStr(req.Context, "table_name")

	switch dsType {
	case "prometheus":
		return fmt.Sprintf(`You are a PromQL expert. The user wants to query Prometheus metrics.

User request: %s

Please use the available tools to explore the metrics and generate the correct PromQL query.
- First use list_metrics to find relevant metrics
- Then use get_metric_labels to understand the label structure
- Finally provide the PromQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the PromQL query>", "explanation": "<brief explanation in the user's language>"}`, req.UserInput)

	default: // SQL-based datasources
		dbContext := ""
		if dbName != "" {
			dbContext += fmt.Sprintf("\nTarget database: %s", dbName)
		}
		if tableName != "" {
			dbContext += fmt.Sprintf("\nTarget table: %s", tableName)
		}

		return fmt.Sprintf(`You are a SQL expert for %s databases. The user wants to query data.
%s
User request: %s

Please use the available tools to explore the database schema and generate the correct SQL query.
- Use list_databases to see available databases
- Use list_tables to see tables in the target database
- Use describe_table to understand the table structure
- Finally provide the SQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the SQL query>", "explanation": "<brief explanation in the user's language>"}`, dsType, dbContext, req.UserInput)
	}
}

func buildQueryGeneratorInputs(req *AIChatRequest) map[string]string {
	inputs := map[string]string{
		"user_input": req.UserInput,
	}
	for _, key := range []string{"datasource_type", "datasource_id", "database_name", "table_name"} {
		if v := ctxStr(req.Context, key); v != "" {
			inputs[key] = v
		}
	}
	// datasource_id may be a number in JSON
	if inputs["datasource_id"] == "" {
		if id := ctxInt64(req.Context, "datasource_id"); id > 0 {
			inputs["datasource_id"] = fmt.Sprintf("%d", id)
		}
	}
	return inputs
}

// --- generic handler ---

func (rt *Router) aiChat(c *gin.Context) {
	if !rt.Center.AIAgent.Enable {
		ginx.Bomb(http.StatusServiceUnavailable, "AI Agent is not enabled")
		return
	}

	var req AIChatRequest
	ginx.BindJSON(c, &req)

	if req.UserInput == "" {
		ginx.Bomb(http.StatusBadRequest, "user_input is required")
		return
	}
	if req.ActionKey == "" {
		ginx.Bomb(http.StatusBadRequest, "action_key is required")
		return
	}
	if req.Context == nil {
		req.Context = make(map[string]interface{})
	}

	handler, ok := actionRegistry[req.ActionKey]
	if !ok {
		ginx.Bomb(http.StatusBadRequest, "unsupported action_key: %s", req.ActionKey)
		return
	}

	logger.Infof("[AIChat] action=%s, user_input=%q", req.ActionKey, truncStr(req.UserInput, 100))

	// Action-specific validation
	if handler.validate != nil {
		if err := handler.validate(&req); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
			return
		}
	}

	// Find AI agent by use_case
	agent, err := models.AIAgentGetByUseCase(rt.Ctx, handler.useCase)
	if err != nil || agent == nil {
		ginx.Bomb(http.StatusBadRequest, "no AI agent configured for use_case=%s", handler.useCase)
		return
	}

	// Resolve LLM config
	llmCfg, err := models.AILLMConfigGetById(rt.Ctx, agent.LLMConfigId)
	if err != nil || llmCfg == nil {
		ginx.Bomb(http.StatusBadRequest, "referenced LLM config not found")
		return
	}
	agent.LLMConfig = llmCfg

	// Select tools
	var tools []aiagent.AgentTool
	if handler.selectTools != nil {
		toolNames := handler.selectTools(&req)
		if toolNames != nil {
			tools = aiagent.GetBuiltinToolDefs(toolNames)
		}
	}

	// Parse extra config
	extraConfig := llmCfg.ExtraConfig

	timeout := 120000
	if extraConfig.TimeoutSeconds > 0 {
		timeout = extraConfig.TimeoutSeconds * 1000
	}

	// Build prompt
	userPrompt := ""
	if handler.buildPrompt != nil {
		userPrompt = handler.buildPrompt(&req)
	}

	// Build workflow inputs
	inputs := map[string]string{"user_input": req.UserInput}
	if handler.buildInputs != nil {
		inputs = handler.buildInputs(&req)
	}

	// Create agent
	agentCfg := aiagent.NewAgent(&aiagent.AIAgentConfig{
		Provider:           llmCfg.APIType,
		LLMURL:             llmCfg.APIURL,
		Model:              llmCfg.Model,
		APIKey:             llmCfg.APIKey,
		Headers:            extraConfig.CustomHeaders,
		AgentMode:          aiagent.AgentModeReAct,
		Tools:              tools,
		Timeout:            timeout,
		Stream:             true,
		UserPromptTemplate: userPrompt,
		SkipSSLVerify:      extraConfig.SkipTLSVerify,
		Proxy:              extraConfig.Proxy,
		Temperature:        extraConfig.Temperature,
		MaxTokens:          extraConfig.MaxTokens,
	})

	// Inject PromClient getter
	aiagent.SetPromClientGetter(func(dsId int64) prom.API {
		return rt.PromClients.GetCli(dsId)
	})

	// Streaming setup
	streamChan := make(chan *models.StreamChunk, 100)
	wfCtx := &models.WorkflowContext{
		Stream:     true,
		StreamChan: streamChan,
		Inputs:     inputs,
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	startTime := time.Now()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[AIChat] PANIC in agent goroutine: %v", r)
				streamChan <- &models.StreamChunk{
					Type:      models.StreamTypeError,
					Content:   fmt.Sprintf("internal error: %v", r),
					Done:      true,
					Timestamp: time.Now().UnixMilli(),
				}
				close(streamChan)
			}
		}()
		_, _, err := agentCfg.Process(rt.Ctx, wfCtx)
		if err != nil {
			logger.Errorf("[AIChat] agent Process error: %v", err)
		}
	}()

	// Stream SSE events
	var accumulatedMessage string
	c.Stream(func(w io.Writer) bool {
		chunk, ok := <-streamChan
		if !ok {
			return false
		}

		data, _ := json.Marshal(chunk)

		if chunk.Type == models.StreamTypeText || chunk.Type == models.StreamTypeThinking {
			if chunk.Delta != "" {
				accumulatedMessage += chunk.Delta
			} else if chunk.Content != "" {
				accumulatedMessage += chunk.Content
			}
		}

		if chunk.Type == models.StreamTypeError {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
			c.Writer.Flush()
			return false
		}

		if chunk.Done || chunk.Type == models.StreamTypeDone {
			doneData := map[string]interface{}{
				"type":        "done",
				"duration_ms": time.Since(startTime).Milliseconds(),
				"message":     accumulatedMessage,
				"response":    chunk.Content,
			}
			finalData, _ := json.Marshal(doneData)
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", finalData)
			c.Writer.Flush()
			return false
		}

		fmt.Fprintf(w, "event: chunk\ndata: %s\n\n", data)
		c.Writer.Flush()
		return true
	})
}

func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
