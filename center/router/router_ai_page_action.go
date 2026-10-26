package router

import (
	"encoding/json"

	"github.com/ccfos/nightingale/v6/models"
)

// mergePageContext retains the page's current state in the generic chat
// context. Page params intentionally stay loosely typed because each page owns
// its own schema; malformed or non-object context is simply unavailable rather
// than turning a chat request into a server error.
func mergePageContext(dst map[string]interface{}, page models.AssistantPageInfo) {
	if page.URL != "" {
		dst["page_url"] = page.URL
	}
	if len(page.Param) == 0 {
		return
	}
	var values map[string]interface{}
	if json.Unmarshal(page.Param, &values) != nil {
		return
	}
	for key, value := range values {
		dst[key] = value
	}
}

// shouldUseExplorerQuery reports whether this turn belongs to the query-dock
// flow: the default chat path plus an explicit explorer-query skill reference.
// The page type is deliberately not part of the decision — the query dock sends
// only page_from.url and page_from.param (no page field), so requiring it would
// silently disable the whole flow.
func shouldUseExplorerQuery(actionKey string, query models.AssistantMessageQuery) bool {
	return actionKey == string(models.ActionKeyGeneralChat) && hasExplorerQueryReference(query)
}

func hasExplorerQueryReference(query models.AssistantMessageQuery) bool {
	for _, name := range query.ExplicitSkillNames() {
		if name == "explorer-query" {
			return true
		}
	}
	return false
}

func pageActionResponse(call *models.AssistantPageActionCall) models.AssistantMessageResponse {
	return models.AssistantMessageResponse{
		ContentType: models.ContentTypePageAction,
		Content:     call.Description,
		Param:       call,
		IsFinish:    true,
		IsFromAI:    true,
	}
}

// pageActionTerminalContent prevents text emitted before the model decided to
// call page_action from becoming the persisted terminal assistant message.
// Stream frames already delivered to a client cannot be withdrawn here.
func pageActionTerminalContent(call *models.AssistantPageActionCall, content string) string {
	if call != nil {
		return ""
	}
	return content
}

// pageActionHistoryContent keeps the requested operation in later turns
// without recording a replayable browser tool call or claiming it executed.
func pageActionHistoryContent(call *models.AssistantPageActionCall) string {
	if call == nil {
		return ""
	}
	args, err := json.Marshal(map[string]interface{}{"name": call.Name, "args": call.Args})
	if err != nil {
		return "A page action was requested; its browser execution result is unavailable."
	}
	return "Requested page action (browser execution result unavailable): " + string(args)
}
