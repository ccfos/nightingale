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

// QueryGeneratorRequest AI query generator request
type QueryGeneratorRequest struct {
	DatasourceType string             `json:"datasource_type"` // prometheus, mysql, doris, ck, pgsql
	DatasourceID   int64              `json:"datasource_id"`
	DatabaseName   string             `json:"database_name,omitempty"`
	TableName      string             `json:"table_name,omitempty"`
	UserInput      string             `json:"user_input"`
	History        []aiagent.ChatMessage `json:"history,omitempty"`
}

func (rt *Router) queryGenerator(c *gin.Context) {
	logger.Infof("[QGen] === Request received ===")

	if !rt.Center.AIAgent.Enable {
		logger.Warningf("[QGen] AI Agent is not enabled, returning 503")
		ginx.Bomb(http.StatusServiceUnavailable, "AI Agent is not enabled")
		return
	}

	var req QueryGeneratorRequest
	ginx.BindJSON(c, &req)
	logger.Infof("[QGen] Request params: datasource_type=%s, datasource_id=%d, database=%s, table=%s, user_input=%q, history_len=%d",
		req.DatasourceType, req.DatasourceID, req.DatabaseName, req.TableName, req.UserInput, len(req.History))

	if req.UserInput == "" {
		ginx.Bomb(http.StatusBadRequest, "user_input is required")
		return
	}
	if req.DatasourceType == "" {
		ginx.Bomb(http.StatusBadRequest, "datasource_type is required")
		return
	}
	if req.DatasourceID == 0 {
		ginx.Bomb(http.StatusBadRequest, "datasource_id is required")
		return
	}

	// Get AI agent by use_case, fallback to default
	agent, err := models.AIAgentGetByUseCase(rt.Ctx, "chat")
	if err != nil || agent == nil {
		// Fallback to legacy is_default
		agent, err = models.AIAgentGetDefault(rt.Ctx)
	}
	if err != nil || agent == nil {
		logger.Errorf("[QGen] No AI agent found for use_case=chat: err=%v, agent=%v", err, agent)
		ginx.Bomb(http.StatusBadRequest, "no AI agent configured for chat")
		return
	}

	// If agent references an LLM config, resolve LLM fields from it
	if agent.LLMConfigId > 0 {
		llmCfg, err := models.AILLMConfigGetById(rt.Ctx, agent.LLMConfigId)
		if err != nil || llmCfg == nil {
			logger.Errorf("[QGen] Failed to get LLM config id=%d: err=%v", agent.LLMConfigId, err)
			ginx.Bomb(http.StatusBadRequest, "referenced LLM config not found")
			return
		}
		agent.APIType = llmCfg.APIType
		agent.APIURL = llmCfg.APIURL
		agent.APIKey = llmCfg.APIKey
		agent.Model = llmCfg.Model
		if llmCfg.ExtraConfig != "" && agent.ExtraConfig == "" {
			agent.ExtraConfig = llmCfg.ExtraConfig
		}
	}
	logger.Infof("[QGen] AI agent: api_type=%s, model=%s, api_url=%s", agent.APIType, agent.Model, agent.APIURL)

	// Build tools based on datasource type
	var builtinToolNames []string
	switch req.DatasourceType {
	case "prometheus":
		builtinToolNames = []string{"list_metrics", "get_metric_labels"}
	case "mysql", "doris", "ck", "clickhouse", "pgsql", "postgresql":
		builtinToolNames = []string{"list_databases", "list_tables", "describe_table"}
	default:
		ginx.Bomb(http.StatusBadRequest, "unsupported datasource_type: %s", req.DatasourceType)
		return
	}

	tools := aiagent.GetBuiltinToolDefs(builtinToolNames)
	logger.Infof("[QGen] Built-in tools: %v, tool_count=%d", builtinToolNames, len(tools))

	// Parse extra config for temperature/max_tokens etc
	var extraConfig struct {
		Temperature    float64 `json:"temperature"`
		MaxTokens      int     `json:"max_tokens"`
		TimeoutSeconds int     `json:"timeout_seconds"`
	}
	if agent.ExtraConfig != "" {
		json.Unmarshal([]byte(agent.ExtraConfig), &extraConfig)
	}

	timeout := 120000 // 120s default
	if extraConfig.TimeoutSeconds > 0 {
		timeout = extraConfig.TimeoutSeconds * 1000
	}

	// Build user prompt with context
	userPrompt := buildQueryGeneratorPrompt(req)
	logger.Infof("[QGen] User prompt built: length=%d, first_200=%q", len(userPrompt), truncStr(userPrompt, 200))

	// Create agent config
	logger.Infof("[QGen] Creating agent: mode=ReAct, stream=true, timeout=%dms", timeout)
	agentCfg := aiagent.NewAgent(&aiagent.AIAgentConfig{
		Provider:           agent.APIType,
		LLMURL:             agent.APIURL,
		Model:              agent.Model,
		APIKey:             agent.APIKey,
		AgentMode:          aiagent.AgentModeReAct,
		Tools:              tools,
		Timeout:            timeout,
		Stream:             true,
		UserPromptTemplate: userPrompt,
	})

	// Inject PromClient getter from Router
	aiagent.SetPromClientGetter(func(dsId int64) prom.API {
		return rt.PromClients.GetCli(dsId)
	})

	// Create WorkflowContext with streaming
	streamChan := make(chan *models.StreamChunk, 100)
	wfCtx := &models.WorkflowContext{
		Stream:     true,
		StreamChan: streamChan,
		Inputs: map[string]string{
			"datasource_type": req.DatasourceType,
			"datasource_id":   fmt.Sprintf("%d", req.DatasourceID),
			"database_name":   req.DatabaseName,
			"table_name":      req.TableName,
			"user_input":      req.UserInput,
		},
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	startTime := time.Now()
	logger.Infof("[QGen] Starting agent goroutine, wfCtx.Inputs=%v", wfCtx.Inputs)

	// Execute agent in goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[QGen] PANIC in agent goroutine: %v", r)
				streamChan <- &models.StreamChunk{
					Type:      models.StreamTypeError,
					Content:   fmt.Sprintf("internal error: %v", r),
					Done:      true,
					Timestamp: time.Now().UnixMilli(),
				}
				close(streamChan)
			}
		}()
		logger.Infof("[QGen] Agent goroutine started, calling Process()")
		result, msg, err := agentCfg.Process(rt.Ctx, wfCtx)
		logger.Infof("[QGen] Agent Process() returned: msg=%q, err=%v, result_stream=%v", msg, err, result != nil && result.Stream)
	}()

	// Stream SSE events
	logger.Infof("[QGen] Starting SSE stream loop")
	chunkCount := 0
	var accumulatedMessage string // accumulate reasoning/thinking text
	c.Stream(func(w io.Writer) bool {
		chunk, ok := <-streamChan
		if !ok {
			logger.Infof("[QGen] StreamChan closed, total chunks=%d, elapsed=%dms", chunkCount, time.Since(startTime).Milliseconds())
			return false
		}

		chunkCount++
		data, _ := json.Marshal(chunk)
		logger.Infof("[QGen] Chunk #%d: type=%s, done=%v, content_len=%d, error=%q",
			chunkCount, chunk.Type, chunk.Done, len(chunk.Content), chunk.Error)

		// Accumulate text/thinking content as reasoning message
		if chunk.Type == models.StreamTypeText || chunk.Type == models.StreamTypeThinking {
			if chunk.Delta != "" {
				accumulatedMessage += chunk.Delta
			} else if chunk.Content != "" {
				accumulatedMessage += chunk.Content
			}
		}

		if chunk.Type == models.StreamTypeError {
			logger.Errorf("[QGen] Sending ERROR event: %s", string(data))
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
			c.Writer.Flush()
			return false
		}

		if chunk.Done || chunk.Type == models.StreamTypeDone {
			// Send the final done event with message (reasoning) and response (final answer) separated
			doneData := map[string]interface{}{
				"type":        "done",
				"duration_ms": time.Since(startTime).Milliseconds(),
				"message":     accumulatedMessage,
				"response":    chunk.Content,
			}
			finalData, _ := json.Marshal(doneData)
			logger.Infof("[QGen] Sending DONE event: duration=%dms, message_len=%d, response_len=%d, response_first200=%q",
				time.Since(startTime).Milliseconds(), len(accumulatedMessage), len(chunk.Content), truncStr(chunk.Content, 200))
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

func buildQueryGeneratorPrompt(req QueryGeneratorRequest) string {
	var prompt string

	switch req.DatasourceType {
	case "prometheus":
		prompt = fmt.Sprintf(`You are a PromQL expert. The user wants to query Prometheus metrics.

User request: %s

Please use the available tools to explore the metrics and generate the correct PromQL query.
- First use list_metrics to find relevant metrics
- Then use get_metric_labels to understand the label structure
- Finally provide the PromQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the PromQL query>", "explanation": "<brief explanation in the user's language>"}`, req.UserInput)

	default: // SQL-based datasources
		dbContext := ""
		if req.DatabaseName != "" {
			dbContext += fmt.Sprintf("\nTarget database: %s", req.DatabaseName)
		}
		if req.TableName != "" {
			dbContext += fmt.Sprintf("\nTarget table: %s", req.TableName)
		}

		prompt = fmt.Sprintf(`You are a SQL expert for %s databases. The user wants to query data.
%s
User request: %s

Please use the available tools to explore the database schema and generate the correct SQL query.
- Use list_databases to see available databases
- Use list_tables to see tables in the target database
- Use describe_table to understand the table structure
- Finally provide the SQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the SQL query>", "explanation": "<brief explanation in the user's language>"}`, req.DatasourceType, dbContext, req.UserInput)
	}

	return prompt
}
