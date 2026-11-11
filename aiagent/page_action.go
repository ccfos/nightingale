package aiagent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

const pageActionToolName = "page_action"

// pageActionTool exposes one fixed function to the model. The name is an enum
// of the actions declared by the current page. The browser validates the
// selected action's arguments when it executes the action.
func pageActionTool(actions []models.AssistantPageAction) AgentTool {
	names := make([]interface{}, 0, len(actions))
	for _, action := range actions {
		names = append(names, action.Name)
	}
	return AgentTool{
		Name:        pageActionToolName,
		Description: "Ask the current browser page to run one declared action. Choose only a name provided by the page. After calling it, stop. This is an execution request; do not claim that the page completed it.",
		Type:        ToolTypeProcessor,
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
					"enum": names,
				},
				"args": map[string]interface{}{
					"type":        "object",
					"description": "Arguments for the selected page action.",
				},
			},
			"required": []string{"name"},
		},
	}
}

func resolvePageActionCall(actions []models.AssistantPageAction, callID, raw string) (*models.AssistantPageActionCall, error) {
	var input struct {
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args"`
	}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, fmt.Errorf("page_action arguments must be JSON: %w", err)
	}
	for _, action := range actions {
		if action.Name != input.Name {
			continue
		}
		if input.Args == nil {
			input.Args = map[string]interface{}{}
		}
		return &models.AssistantPageActionCall{
			CallID:      callID,
			Name:        action.Name,
			Description: action.Description,
			Args:        input.Args,
		}, nil
	}
	return nil, fmt.Errorf("page action %q was not declared by this request", input.Name)
}

func emitPageAction(config *ToolLoopConfig, call *models.AssistantPageActionCall) {
	if config.StreamChan == nil || call == nil {
		return
	}
	config.StreamChan <- &StreamChunk{
		Type:       StreamTypePageAction,
		PageAction: call,
		RequestID:  config.RequestID,
		Timestamp:  time.Now().UnixMilli(),
	}
}

// pageActionSystemPrompt gives the model the page-owned action schemas. The
// tool definition constrains the action name; the selected action's argument
// shape remains in this per-turn prompt, exactly as the browser declared it.
func pageActionSystemPrompt(actions []models.AssistantPageAction) string {
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Page actions\n")
	b.WriteString("The current page can run the actions below. To request one, call page_action with its name and args matching its input schema. The call ends the current turn and the browser executes it. Do not claim that the page executed the action or that it returned a result.\n")
	for _, action := range actions {
		b.WriteString("- ")
		b.WriteString(action.Name)
		b.WriteString(": ")
		b.WriteString(action.Description)
		b.WriteString("\n  Input schema: ")
		b.Write(action.InputSchema)
		b.WriteString("\n")
	}
	return b.String()
}
