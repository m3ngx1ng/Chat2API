package completions

import (
	"strings"
	"testing"
)

func TestBuildChatRequestCollapsesStatelessHistory(t *testing.T) {
	apiReq := &ApiReq{
		Messages: []ApiMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello, how can I help?"},
			{Role: "user", Content: "create hello.py"},
		},
		Model: "auto",
	}
	chatReq := BuildChatRequest(apiReq)
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected collapsed single user message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Author.Role != "user" {
		t.Fatalf("expected collapsed message role user, got %q", chatReq.Messages[0].Author.Role)
	}
	parts := chatReq.Messages[0].Content.Parts
	if len(parts) != 1 {
		t.Fatalf("expected single text part, got %#v", parts)
	}
	text, ok := parts[0].(string)
	if !ok {
		t.Fatalf("expected collapsed text content, got %#v", parts[0])
	}
	if text == "" || !containsAll(text, "User:\nhi", "Assistant:\nhello, how can I help?", "User:\ncreate hello.py") {
		t.Fatalf("collapsed transcript missing expected history, got %q", text)
	}
	if strings.Contains(text, "Assistant:\nhello, how can I help?\n\nAssistant:") {
		t.Fatalf("unexpected duplicated assistant transcript in %q", text)
	}
}

func TestBuildChatRequestKeepsConversationModeWhenConversationIDPresent(t *testing.T) {
	apiReq := &ApiReq{
		ConversationId: "conv_123",
		Messages: []ApiMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
			{Role: "user", Content: "continue"},
		},
		Model: "auto",
	}
	chatReq := BuildChatRequest(apiReq)
	if len(chatReq.Messages) != 3 {
		t.Fatalf("expected original messages to be preserved with conversation_id, got %d", len(chatReq.Messages))
	}
	if chatReq.ConversationId != "conv_123" {
		t.Fatalf("expected conversation id to be preserved, got %q", chatReq.ConversationId)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if part == "" {
			continue
		}
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
