package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DefaultGeminiURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

// Gemini implements the LLM interface for Google Gemini API
type Gemini struct {
	config *Config
	client *http.Client
}

// NewGemini creates a new Gemini provider
func NewGemini(cfg *Config, client *http.Client) (*Gemini, error) {
	cfg.BaseURL = NormalizeGeminiBase(cfg.BaseURL)
	return &Gemini{
		config: cfg,
		client: client,
	}, nil
}

func (g *Gemini) Name() string {
	return ProviderGemini
}

// Gemini API request/response structures
type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     float64  `json:"temperature,omitempty"`
	TopP            float64  `json:"topP,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content       geminiContent `json:"content"`
		FinishReason  string        `json:"finishReason"`
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings,omitempty"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (g *Gemini) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	geminiReq := g.convertRequest(req)

	url := g.buildURL(false)
	respBody, err := g.doRequest(ctx, url, geminiReq)
	if err != nil {
		return nil, err
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("Gemini API error: %s", geminiResp.Error.Message)
	}

	return g.convertResponse(&geminiResp), nil
}

func (g *Gemini) GenerateStream(ctx context.Context, req *GenerateRequest) (<-chan StreamChunk, error) {
	geminiReq := g.convertRequest(req)

	jsonData, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := g.buildURL(true)
	resp, err := doHTTPStreamWithRetry(ctx, g.client, "Gemini",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		},
		g.setHeaders,
	)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk, 100)
	go g.streamResponse(ctx, resp, ch)
	return ch, nil
}

func (g *Gemini) streamResponse(ctx context.Context, resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var buffer strings.Builder

	for {
		select {
		case <-ctx.Done():
			ch <- StreamChunk{Done: true, Error: ctx.Err()}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				ch <- StreamChunk{Done: true, Error: err}
			} else {
				ch <- StreamChunk{Done: true}
			}
			return
		}

		line = strings.TrimSpace(line)

		// Gemini streams JSON objects, accumulate until we have a complete one
		if line == "" {
			continue
		}

		// Handle SSE format if present
		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")
		}

		buffer.WriteString(line)

		// Try to parse accumulated JSON
		var geminiResp geminiResponse
		if err := json.Unmarshal([]byte(buffer.String()), &geminiResp); err != nil {
			// Not complete yet, continue accumulating
			continue
		}

		// Reset buffer for next response
		buffer.Reset()

		if len(geminiResp.Candidates) > 0 {
			candidate := geminiResp.Candidates[0]
			chunk := StreamChunk{
				FinishReason: candidate.FinishReason,
			}

			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					chunk.Content += part.Text
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					chunk.ToolCalls = append(chunk.ToolCalls, ToolCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					})
				}
			}

			ch <- chunk

			if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
				ch <- StreamChunk{Done: true}
				return
			}
		}
	}
}

func (g *Gemini) convertRequest(req *GenerateRequest) *geminiRequest {
	genCfg := &geminiGenerationConfig{
		TopP:          req.TopP,
		StopSequences: req.Stop,
	}

	switch {
	case req.Temperature != nil:
		genCfg.Temperature = *req.Temperature
	case g.config.Temperature != nil:
		genCfg.Temperature = *g.config.Temperature
	}

	switch {
	case req.MaxTokens != nil:
		genCfg.MaxOutputTokens = *req.MaxTokens
	case g.config.MaxTokens != nil:
		genCfg.MaxOutputTokens = *g.config.MaxTokens
	}

	geminiReq := &geminiRequest{
		GenerationConfig: genCfg,
	}

	// Convert messages
	for _, msg := range req.Messages {
		if msg.Role == RoleSystem {
			geminiReq.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			continue
		}

		// Map roles
		role := msg.Role
		if role == RoleAssistant {
			role = "model"
		}

		geminiReq.Contents = append(geminiReq.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var declarations []geminiFunctionDeclaration
		for _, tool := range req.Tools {
			declarations = append(declarations, geminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			})
		}
		geminiReq.Tools = []geminiTool{{FunctionDeclarations: declarations}}
	}

	return geminiReq
}

func (g *Gemini) convertResponse(resp *geminiResponse) *GenerateResponse {
	result := &GenerateResponse{}

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		result.FinishReason = candidate.FinishReason

		var textParts []string
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				})
			}
		}
		result.Content = strings.Join(textParts, "")
	}

	if resp.UsageMetadata != nil {
		result.Usage = &Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	return result
}

func (g *Gemini) buildURL(stream bool) string {
	action := "generateContent"
	if stream {
		action = "streamGenerateContent"
	}

	// Check if baseURL already contains the full path
	if strings.Contains(g.config.BaseURL, ":generateContent") ||
		strings.Contains(g.config.BaseURL, ":streamGenerateContent") {
		return fmt.Sprintf("%s?key=%s", g.config.BaseURL, g.config.APIKey)
	}

	return fmt.Sprintf("%s/%s:%s?key=%s",
		g.config.BaseURL,
		g.config.Model,
		action,
		g.config.APIKey,
	)
}

func (g *Gemini) doRequest(ctx context.Context, url string, req *geminiRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return doHTTPWithRetry(ctx, g.client, "Gemini",
		func() (*http.Request, error) {
			return http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		},
		g.setHeaders,
	)
}

func (g *Gemini) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	for k, v := range g.config.Headers {
		req.Header.Set(k, v)
	}
}
