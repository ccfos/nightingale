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
