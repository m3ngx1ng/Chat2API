package service

import (
	"chat2api/app/types/completions"
	"chat2api/app/types/responses"
	"encoding/json"
	"strings"
)

func completionMessagesFromResponse(req *responses.ApiReq) []completions.ApiMessage {
	messages := make([]completions.ApiMessage, 0)
	if strings.TrimSpace(req.Instructions) != "" {
		messages = append(messages, completions.ApiMessage{Role: "system", Content: strings.TrimSpace(req.Instructions)})
	}
	return append(messages, completionMessagesFromResponseInput(req.Input)...)
}

func completionMessagesFromResponseInput(input interface{}) []completions.ApiMessage {
	switch v := input.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []completions.ApiMessage{{Role: "user", Content: strings.TrimSpace(v)}}
	case map[string]interface{}:
		if message, ok := responseInputMessage(v); ok {
			return []completions.ApiMessage{message}
		}
		return nil
	case []interface{}:
		messages := make([]completions.ApiMessage, 0, len(v))
		for _, item := range v {
			switch part := item.(type) {
			case string:
				if text := strings.TrimSpace(part); text != "" {
					messages = append(messages, completions.ApiMessage{Role: "user", Content: text})
				}
			case map[string]interface{}:
				if message, ok := responseInputMessage(part); ok {
					messages = append(messages, message)
				}
			}
		}
		return messages
	default:
		return nil
	}
}

func responseInputMessage(item map[string]interface{}) (completions.ApiMessage, bool) {
	switch responseStringValue(item["type"], "") {
	case "function_call":
		return responseFunctionCallMessage(item)
	case "function_call_output":
		return responseFunctionCallOutputMessage(item)
	}
	role := responseInputRole(item)
	if role == "" {
		return completions.ApiMessage{}, false
	}
	content := responseMessageContent(item)
	if !responseContentHasValue(content) {
		return completions.ApiMessage{}, false
	}
	return completions.ApiMessage{Role: role, Content: content}, true
}

func responseFunctionCallMessage(item map[string]interface{}) (completions.ApiMessage, bool) {
	name := strings.TrimSpace(responseStringValue(item["name"], ""))
	if name == "" {
		return completions.ApiMessage{}, false
	}
	callID := strings.TrimSpace(responseStringValue(item["call_id"], responseStringValue(item["id"], "")))
	arguments := responseStructuredText(item["arguments"])
	return completions.ApiMessage{
		Role: "assistant",
		ToolCalls: []completions.ToolCall{{
			ID:   callID,
			Type: "function",
			Function: completions.ToolCallFunction{
				Name:      name,
				Arguments: arguments,
			},
		}},
	}, true
}

func responseFunctionCallOutputMessage(item map[string]interface{}) (completions.ApiMessage, bool) {
	callID := strings.TrimSpace(responseStringValue(item["call_id"], ""))
	if callID == "" {
		return completions.ApiMessage{}, false
	}
	content := responseFunctionOutputContent(item)
	if !responseContentHasValue(content) {
		return completions.ApiMessage{}, false
	}
	return completions.ApiMessage{Role: "tool", ToolCallID: callID, Content: content}, true
}

func responseFunctionOutputContent(item map[string]interface{}) interface{} {
	for _, key := range []string{"output", "result", "content", "text"} {
		value, ok := item[key]
		if !ok {
			continue
		}
		if text := responseStructuredText(value); text != "" {
			return text
		}
	}
	return ""
}

func responseStructuredText(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
}

func responseInputRole(item map[string]interface{}) string {
	if role := normalizeResponseInputRole(responseStringValue(item["role"], "")); role != "" {
		return role
	}
	switch responseStringValue(item["type"], "") {
	case "input_text", "input_image", "text", "input_file":
		return "user"
	case "output_text", "refusal":
		return "assistant"
	case "message":
		return inferResponseRoleFromContent(item["content"])
	default:
		return ""
	}
}

func normalizeResponseInputRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "system", "developer", "user", "assistant", "tool", "function":
		return role
	default:
		return ""
	}
}

func inferResponseRoleFromContent(content interface{}) string {
	parts, ok := content.([]interface{})
	if !ok {
		return ""
	}
	for _, raw := range parts {
		part, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch responseStringValue(part["type"], "") {
		case "output_text", "refusal":
			return "assistant"
		case "input_text", "input_image", "text", "input_file":
			return "user"
		}
	}
	return ""
}

func responseMessageContent(item map[string]interface{}) interface{} {
	if content, ok := item["content"].([]interface{}); ok {
		return content
	}
	return responseMessageContentText(item)
}

func responseContentHasValue(content interface{}) bool {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return responseContentPartsHaveValue(v)
	default:
		return v != nil
	}
}

func responseContentPartsHaveValue(parts []interface{}) bool {
	for _, raw := range parts {
		switch part := raw.(type) {
		case string:
			if strings.TrimSpace(part) != "" {
				return true
			}
		case map[string]interface{}:
			switch responseStringValue(part["type"], "") {
			case "text", "input_text", "output_text", "refusal":
				if strings.TrimSpace(responseStringValue(part["text"], "")) != "" {
					return true
				}
			case "image_url", "input_image", "image", "input_file":
				return true
			}
		}
	}
	return false
}

func responseMessageContentText(item map[string]interface{}) string {
	if text := responseStringValue(item["text"], ""); text != "" {
		return text
	}
	if text := responseStringValue(item["content"], ""); text != "" {
		return text
	}
	if content, ok := item["content"].([]interface{}); ok {
		parts := make([]string, 0, len(content))
		for _, raw := range content {
			if part, ok := raw.(map[string]interface{}); ok {
				if text := responseStringValue(part["text"], ""); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	}
	if isResponsesContentPart(item) {
		return responseStringValue(item["text"], "")
	}
	return ""
}

func isResponsesContentPart(item map[string]interface{}) bool {
	switch responseStringValue(item["type"], "") {
	case "text", "input_text", "output_text":
		return true
	default:
		return false
	}
}

func hasResponsesImageGenerationTool(req *responses.ApiReq) bool {
	if responseToolChoiceType(req.ToolChoice) == "image_generation" {
		return true
	}
	tools := req.Tools
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) == "image_generation" {
			return true
		}
	}
	return false
}

func completionToolsFromResponses(tools []responses.Tool) []completions.Tool {
	out := make([]completions.Tool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "function" {
			continue
		}
		out = append(out, completions.Tool{
			Type: "function",
			Function: completions.ToolFunction{
				Name:        strings.TrimSpace(tool.Name),
				Description: tool.Description,
				Parameters:  tool.Parameters,
				Strict:      tool.Strict,
			},
		})
	}
	return out
}

func completionToolChoiceFromResponses(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		if strings.TrimSpace(responseStringValue(v["type"], "")) == "function" {
			if _, ok := v["function"]; ok {
				return v
			}
			if name := strings.TrimSpace(responseStringValue(v["name"], "")); name != "" {
				return map[string]interface{}{
					"type":     "function",
					"function": map[string]interface{}{"name": name},
				}
			}
		}
	}
	return value
}

func responseToolChoiceType(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		return strings.TrimSpace(responseStringValue(v["type"], ""))
	default:
		return ""
	}
}

func hasResponsesNonImageTools(tools []responses.Tool) bool {
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "" && strings.TrimSpace(tool.Type) != "image_generation" {
			return true
		}
	}
	return false
}

func responseStringValue(value interface{}, fallback string) string {
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}
