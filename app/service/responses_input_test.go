package service

import "testing"

func TestResponseInputMessageInfersAssistantRoleFromOutputText(t *testing.T) {
	message, ok := responseInputMessage(map[string]interface{}{
		"type": "message",
		"content": []interface{}{
			map[string]interface{}{"type": "output_text", "text": "hello from assistant"},
		},
	})
	if !ok {
		t.Fatal("expected assistant output_text message to be accepted")
	}
	if message.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", message.Role)
	}
	content, ok := message.Content.([]interface{})
	if !ok || len(content) != 1 {
		t.Fatalf("expected assistant content parts, got %#v", message.Content)
	}
	part, ok := content[0].(map[string]interface{})
	if !ok || responseStringValue(part["text"], "") != "hello from assistant" {
		t.Fatalf("expected assistant output_text content, got %#v", message.Content)
	}
}

func TestResponseInputMessageMapsFunctionCallOutputToToolMessage(t *testing.T) {
	message, ok := responseInputMessage(map[string]interface{}{
		"type":    "function_call_output",
		"call_id": "call_123",
		"output": map[string]interface{}{
			"status": "ok",
			"value":  "done",
		},
	})
	if !ok {
		t.Fatal("expected function_call_output message to be accepted")
	}
	if message.Role != "tool" {
		t.Fatalf("expected tool role, got %q", message.Role)
	}
	if message.ToolCallID != "call_123" {
		t.Fatalf("expected tool_call_id to be preserved, got %q", message.ToolCallID)
	}
	text, ok := message.Content.(string)
	if !ok || text == "" {
		t.Fatalf("expected serialized tool output content, got %#v", message.Content)
	}
}

func TestResponseInputMessageDropsUnknownIntermediateItem(t *testing.T) {
	_, ok := responseInputMessage(map[string]interface{}{
		"type":   "reasoning",
		"status": "completed",
		"id":     "rs_123",
	})
	if ok {
		t.Fatal("expected unknown intermediate item to be ignored")
	}
}
