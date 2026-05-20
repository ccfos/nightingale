package airunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	_ "github.com/ccfos/nightingale/v6/aiagent/tools"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

const (
	ProcessorTypeAIRunner         = "ai_runner"
	DefaultAIRunnerTimeoutSeconds = 180
	AIRunnerOutputField           = "ai_runner_result"
	setEventAnnotationTool        = "set_event_annotation"
)

type aiRunnerSettings struct {
	LLMConfigID    int64  `json:"llm_config_id"`
	Description    string `json:"description"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func init() {
	models.RegisterProcessor(ProcessorTypeAIRunner, &ProcessorAdapter{})
}

type ProcessorAdapter struct {
	settings aiRunnerSettings
}

func (p *ProcessorAdapter) Init(settings interface{}) (models.Processor, error) {
	b, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var s aiRunnerSettings
	if err = json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.LLMConfigID <= 0 {
		return nil, errors.New("ai_runner: llm_config_id is required")
	}
	s.Description = strings.TrimSpace(s.Description)
	if s.Description == "" {
		return nil, errors.New("ai_runner: description is required")
	}
	if s.TimeoutSeconds <= 0 {
		s.TimeoutSeconds = DefaultAIRunnerTimeoutSeconds
	}
	return &ProcessorAdapter{settings: s}, nil
}

func (p *ProcessorAdapter) Process(ctxObj *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	deps, skillsPath := GetRuntime()
	if deps == nil {
		return wfCtx, "", errors.New("ai_runner: runtime not initialized on this process")
	}

	llmCfg, err := p.resolveLLMConfig(ctxObj)
	if err != nil {
		return wfCtx, "", err
	}

	agent := p.buildAgent(llmCfg, deps, skillsPath)
	agent.SetExternalToolHandler(p.handleExternalTool(ctxObj, wfCtx, agent))

	resp, err := agent.Run(context.Background(), workflowContextToRequest(wfCtx))
	if err != nil {
		return wfCtx, "", fmt.Errorf("ai_runner: agent run failed: %w", err)
	}
	if resp != nil && resp.Error != "" {
		return wfCtx, "", fmt.Errorf("ai_runner: %s", resp.Error)
	}

	if wfCtx.Event == nil && resp != nil {
		if wfCtx.Output == nil {
			wfCtx.Output = make(map[string]interface{})
		}
		wfCtx.Output[AIRunnerOutputField] = resp.Content
	}

	if resp == nil {
		return wfCtx, "", nil
	}
	return wfCtx, fmt.Sprintf("AI Runner completed: %d iterations, success=%v", resp.Iterations, resp.Success), nil
}

func (p *ProcessorAdapter) resolveLLMConfig(ctxObj *ctx.Context) (*llm.Config, error) {
	c, err := models.AILLMConfigGetById(ctxObj, p.settings.LLMConfigID)
	if err != nil {
		return nil, fmt.Errorf("ai_runner: load LLM config id=%d failed: %v", p.settings.LLMConfigID, err)
	}
	if c == nil {
		return nil, fmt.Errorf("ai_runner: LLM config id=%d not found", p.settings.LLMConfigID)
	}
	if !c.Enabled {
		return nil, fmt.Errorf("ai_runner: LLM config id=%d is disabled", p.settings.LLMConfigID)
	}
	extra := c.ExtraConfig
	return &llm.Config{
		Provider:      c.APIType,
		BaseURL:       c.APIURL,
		APIKey:        c.APIKey,
		Model:         c.Model,
		Headers:       extra.CustomHeaders,
		Timeout:       p.settings.TimeoutSeconds * 1000,
		SkipSSLVerify: extra.SkipTLSVerify,
		Proxy:         extra.Proxy,
		Temperature:   extra.Temperature,
		MaxTokens:     extra.MaxTokens,
		ExtraBody:     extra.CustomParams,
	}, nil
}

func (p *ProcessorAdapter) buildAgent(llmCfg *llm.Config, deps *aiagent.ToolDeps, skillsPath string) *aiagent.Agent {
	cfg := &aiagent.AgentConfig{
		AgentMode:          aiagent.AgentModeReAct,
		Timeout:            p.settings.TimeoutSeconds * 1000,
		UserPromptTemplate: p.settings.Description,
		OutputField:        AIRunnerOutputField,
		Skills:             &aiagent.SkillConfig{AutoSelect: true, MaxSkills: 2},
		Tools:              []aiagent.AgentTool{setEventAnnotationToolDef()},
	}
	// 浅拷贝 deps：InitSkills 会写 SkillsPath，并发 Process 不能共享同一指针
	depsCopy := *deps
	agent := aiagent.NewAgent(cfg, aiagent.WithLLMConfig(llmCfg), aiagent.WithToolDeps(&depsCopy))
	if skillsPath != "" {
		agent.InitSkills(skillsPath)
	}
	return agent
}

func (p *ProcessorAdapter) handleExternalTool(ctxObj *ctx.Context, wfCtx *models.WorkflowContext, agent *aiagent.Agent) aiagent.ExternalToolHandler {
	return func(_ context.Context, tool *aiagent.AgentTool, args map[string]interface{}, _ *aiagent.AgentRequest) (string, error) {
		if tool.Name == setEventAnnotationTool {
			return handleSetEventAnnotation(args, wfCtx)
		}
		if tool.Type == aiagent.ToolTypeSkill {
			return executeSkillTool(ctxObj, tool, args, wfCtx, agent.SkillRegistry())
		}
		return "", fmt.Errorf("ai_runner: unsupported external tool: name=%s type=%s", tool.Name, tool.Type)
	}
}
