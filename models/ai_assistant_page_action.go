package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AssistantMessageReference is an explicit resource selected by the page. At
// present the chat runtime consumes only type=skill; other reference types are
// retained in the message unchanged for forward compatibility.
type AssistantMessageReference struct {
	ID    string                  `json:"id,omitempty"`
	Name  string                  `json:"name,omitempty"`
	Type  string                  `json:"type,omitempty"`
	Skill AssistantSkillReference `json:"skill,omitempty"`
}

// AssistantSkillReference identifies an explicitly selected built-in skill.
type AssistantSkillReference struct {
	Name string `json:"name,omitempty"`
}

// ExplicitSkillNames returns exactly the skills selected by the client. It
// deliberately does not infer skills from text or page information.
func (q AssistantMessageQuery) ExplicitSkillNames() []string {
	seen := make(map[string]struct{})
	var names []string
	for _, ref := range q.References {
		if ref.Type != "skill" {
			continue
		}
		name := strings.TrimSpace(ref.Skill.Name)
		if name == "" {
			name = strings.TrimSpace(ref.Name)
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// AssistantPageAction is an action implemented by the active browser page.
// InputSchema is normalized to the small JSON Schema subset advertised to the
// model. The browser remains the authority that validates arguments when it
// executes the action.
type AssistantPageAction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// AssistantPageActionCall is the completed server-to-page command carried in
// an AssistantMessageResponse whose content_type is page_action.
type AssistantPageActionCall struct {
	CallID      string                 `json:"call_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Args        map[string]interface{} `json:"args"`
}

// ValidPageActions retains only declarations that can safely be shown to the
// model. A bad browser declaration must not fail an otherwise ordinary chat
// request; it simply cannot be selected as a page action in that turn.
func ValidPageActions(actions []AssistantPageAction) []AssistantPageAction {
	const maxActions = 16
	const maxDescriptionRunes = 500
	const maxNameBytes = 64
	const maxSchemaBytes = 8 * 1024

	valid := make([]AssistantPageAction, 0, min(len(actions), maxActions))
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if len(valid) == maxActions {
			break
		}
		action.Name = strings.TrimSpace(action.Name)
		action.Description = strings.TrimSpace(action.Description)
		if action.Description == "" || len(action.Name) > maxNameBytes || !validPageActionName(action.Name) {
			continue
		}
		if len([]rune(action.Description)) > maxDescriptionRunes {
			action.Description = string([]rune(action.Description)[:maxDescriptionRunes])
		}
		if len(action.InputSchema) > maxSchemaBytes {
			continue
		}
		if len(action.InputSchema) == 0 {
			action.InputSchema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		normalized, err := normalizePageActionSchema(action.InputSchema)
		if err != nil {
			continue
		}
		action.InputSchema = normalized
		if _, exists := seen[action.Name]; exists {
			continue
		}
		seen[action.Name] = struct{}{}
		valid = append(valid, action)
	}
	return valid
}

func validPageActionName(name string) bool {
	if name == "" || !isASCIIAlpha(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		char := name[i]
		if !(isASCIIAlpha(char) || (char >= '0' && char <= '9') || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

func isASCIIAlpha(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

var pageActionPropertyTypes = map[string]struct{}{
	"string": {}, "number": {}, "integer": {}, "boolean": {}, "array": {}, "object": {},
}

// normalizePageActionSchema accepts only the JSON Schema subset which can be
// safely embedded in the page-action prompt. This is intentionally a
// declaration filter, not server-side argument validation: the browser owns
// execution and validates args against its full action contract.
func normalizePageActionSchema(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		if err == nil {
			err = fmt.Errorf("schema must be an object")
		}
		return nil, fmt.Errorf("invalid page action schema: %w", err)
	}
	for key := range schema {
		switch key {
		case "type", "properties", "required", "additionalProperties":
		default:
			return nil, fmt.Errorf("schema key %q is not allowed", key)
		}
	}
	if kind, ok := schema["type"]; ok && kind != "object" {
		return nil, fmt.Errorf("schema type must be object, got %v", kind)
	}

	result := map[string]interface{}{"type": "object"}
	if value, ok := schema["additionalProperties"]; ok {
		if _, valid := value.(bool); !valid {
			return nil, fmt.Errorf("additionalProperties must be a boolean")
		}
		result["additionalProperties"] = value
	}

	properties := map[string]interface{}{}
	if value, ok := schema["properties"]; ok {
		var valid bool
		properties, valid = value.(map[string]interface{})
		if !valid {
			return nil, fmt.Errorf("properties must be an object")
		}
	}
	cleanProperties := make(map[string]interface{}, len(properties))
	for name, value := range properties {
		clean, err := normalizePageActionProperty(name, value)
		if err != nil {
			return nil, err
		}
		cleanProperties[name] = clean
	}
	result["properties"] = cleanProperties

	if value, ok := schema["required"]; ok {
		required, err := normalizePageActionRequired("schema", value, cleanProperties)
		if err != nil {
			return nil, err
		}
		result["required"] = required
	}

	normalized, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized schema: %w", err)
	}
	return normalized, nil
}

func normalizePageActionProperty(name string, value interface{}) (map[string]interface{}, error) {
	property, valid := value.(map[string]interface{})
	if !valid {
		return nil, fmt.Errorf("property %q must be an object", name)
	}
	result := make(map[string]interface{}, len(property))
	for key, value := range property {
		switch key {
		case "type":
			kind, isString := value.(string)
			if _, known := pageActionPropertyTypes[kind]; !isString || !known {
				return nil, fmt.Errorf("property %q has unsupported type %v", name, value)
			}
			result[key] = kind
		case "description":
			if _, valid := value.(string); !valid {
				return nil, fmt.Errorf("property %q description must be a string", name)
			}
			result[key] = value
		case "enum":
			values, valid := value.([]interface{})
			if !valid || len(values) == 0 {
				return nil, fmt.Errorf("property %q enum must be a non-empty array", name)
			}
			for _, enum := range values {
				switch enum.(type) {
				case string, float64, bool:
				default:
					return nil, fmt.Errorf("property %q enum values must be scalars", name)
				}
			}
			result[key] = values
		case "properties":
			children, valid := value.(map[string]interface{})
			if !valid {
				return nil, fmt.Errorf("property %q properties must be an object", name)
			}
			cleanChildren := make(map[string]interface{}, len(children))
			for childName, child := range children {
				clean, err := normalizePageActionProperty(name+"."+childName, child)
				if err != nil {
					return nil, err
				}
				cleanChildren[childName] = clean
			}
			result[key] = cleanChildren
		case "required":
			// Validate it after properties are normalized, independent of JSON map order.
		case "items":
			clean, err := normalizePageActionProperty(name+"[]", value)
			if err != nil {
				return nil, err
			}
			result[key] = clean
		default:
			return nil, fmt.Errorf("property %q key %q is not allowed", name, key)
		}
	}
	if value, ok := property["required"]; ok {
		children, _ := result["properties"].(map[string]interface{})
		required, err := normalizePageActionRequired("property "+name, value, children)
		if err != nil {
			return nil, err
		}
		result["required"] = required
	}
	return result, nil
}

func normalizePageActionRequired(owner string, value interface{}, properties map[string]interface{}) ([]interface{}, error) {
	values, valid := value.([]interface{})
	if !valid {
		return nil, fmt.Errorf("%s required must be an array", owner)
	}
	for _, value := range values {
		name, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("%s required entries must be strings", owner)
		}
		if _, declared := properties[name]; !declared {
			return nil, fmt.Errorf("%s requires %q, which is not in properties", owner, name)
		}
	}
	return values, nil
}
