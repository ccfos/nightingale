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
	if !rt.Center.AIAgent.Enable {
		ginx.Bomb(http.StatusServiceUnavailable, "AI Agent is not enabled")
		return
	}

	var req QueryGeneratorRequest
	ginx.BindJSON(c, &req)

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

	// Get default LLM provider
	provider, err := models.LLMProviderGetDefault(rt.Ctx)
	if err != nil || provider == nil {
		ginx.Bomb(http.StatusBadRequest, "no default LLM provider configured")
		return
	}

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

	// Parse extra config for temperature/max_tokens etc
	var extraConfig struct {
		Temperature    float64 `json:"temperature"`
		MaxTokens      int     `json:"max_tokens"`
		TimeoutSeconds int     `json:"timeout_seconds"`
	}
	if provider.ExtraConfig != "" {
		json.Unmarshal([]byte(provider.ExtraConfig), &extraConfig)
	}

	timeout := 120000 // 120s default
	if extraConfig.TimeoutSeconds > 0 {
		timeout = extraConfig.TimeoutSeconds * 1000
	}

	// Create agent config
	agentCfg := aiagent.NewAgent(&aiagent.AIAgentConfig{
		Provider:  provider.APIType,
		LLMURL:    provider.APIURL,
		Model:     provider.Model,
		APIKey:    provider.APIKey,
		AgentMode: aiagent.AgentModeReAct,
		Tools:     tools,
		Timeout:   timeout,
		Stream:    true,
	})

	// Inject PromClient getter from Router
	aiagent.SetPromClientGetter(func(dsId int64) prom.API {
		return rt.PromClients.GetCli(dsId)
	})

	// Build user prompt with context
	userMessage := buildQueryGeneratorPrompt(req)

	// Build conversation history
	messages := make([]aiagent.ChatMessage, 0, len(req.History)+1)
	for _, h := range req.History {
		messages = append(messages, h)
	}
	messages = append(messages, aiagent.ChatMessage{
		Role:    "user",
		Content: userMessage,
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

	// Execute agent in goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("query-generator panic: %v", r)
				streamChan <- &models.StreamChunk{
					Type:      models.StreamTypeError,
					Content:   fmt.Sprintf("internal error: %v", r),
					Done:      true,
					Timestamp: time.Now().UnixMilli(),
				}
				close(streamChan)
			}
		}()
		agentCfg.Process(rt.Ctx, wfCtx)
	}()

	// Stream SSE events
	c.Stream(func(w io.Writer) bool {
		chunk, ok := <-streamChan
		if !ok {
			return false
		}

		data, _ := json.Marshal(chunk)

		if chunk.Done || chunk.Type == models.StreamTypeDone {
			// Send the final done event with duration
			doneData := map[string]interface{}{
				"type":        "done",
				"duration_ms": time.Since(startTime).Milliseconds(),
				"content":     chunk.Content,
			}
			finalData, _ := json.Marshal(doneData)
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", finalData)
			c.Writer.Flush()
			return false
		}

		if chunk.Type == models.StreamTypeError {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
			c.Writer.Flush()
			return false
		}

		fmt.Fprintf(w, "event: chunk\ndata: %s\n\n", data)
		c.Writer.Flush()
		return true
	})
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
