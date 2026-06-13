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

func TestBuildChatRequestSanitizesInlineImageDataURLInTextHistory(t *testing.T) {
	apiReq := &ApiReq{
		ConversationId: "conv_123",
		Messages: []ApiMessage{
			{Role: "assistant", Content: "![image](data:image/png;base64,AAAAABBBBBCCCCCDDDD==)"},
			{Role: "user", Content: "继续描述刚才那张图"},
		},
		Model: "auto",
	}
	chatReq := BuildChatRequest(apiReq)
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chatReq.Messages))
	}
	parts := chatReq.Messages[0].Content.Parts
	if len(parts) != 1 {
		t.Fatalf("expected single text part, got %#v", parts)
	}
	text, ok := parts[0].(string)
	if !ok {
		t.Fatalf("expected string text part, got %#v", parts[0])
	}
	if strings.Contains(text, "data:image/png;base64") {
		t.Fatalf("expected inline image data url to be sanitized, got %q", text)
	}
	if !strings.Contains(text, "[image omitted") {
		t.Fatalf("expected placeholder after sanitization, got %q", text)
	}
	userParts := chatReq.Messages[1].Content.Parts
	if len(userParts) < 2 {
		t.Fatalf("expected carried image plus user text, got %#v", userParts)
	}
	imagePart, ok := userParts[0].(map[string]interface{})
	if !ok || imagePart["type"] != "input_image" {
		t.Fatalf("expected carried inline image to become input_image, got %#v", userParts[0])
	}
}

func TestBuildChatRequestKeepsStructuredInputImage(t *testing.T) {
	apiReq := &ApiReq{
		Messages: []ApiMessage{{
			Role: "user",
			Content: []interface{}{
				map[string]interface{}{"type": "input_text", "text": "edit this"},
				map[string]interface{}{"type": "input_image", "image_url": "data:image/png;base64,AAAAABBBBBCCCCCDDDD=="},
			},
		}},
		Model: "auto",
	}
	chatReq := BuildChatRequest(apiReq)
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	parts := chatReq.Messages[0].Content.Parts
	if len(parts) != 2 {
		t.Fatalf("expected multimodal parts, got %#v", parts)
	}
	imagePart, ok := parts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first part to be image object, got %#v", parts[0])
	}
	if got := imagePart["image_url"]; got != "data:image/png;base64,AAAAABBBBBCCCCCDDDD==" {
		t.Fatalf("expected structured input image to be preserved, got %#v", got)
	}
}

func TestBuildChatRequestCollapsedHistoryCarriesAssistantImageToLatestUser(t *testing.T) {
	apiReq := &ApiReq{
		Messages: []ApiMessage{
			{Role: "user", Content: "先生成图片"},
			{Role: "assistant", Content: "![image](data:image/png;base64,AAAAABBBBBCCCCCDDDD==)"},
			{Role: "user", Content: "把刚才那张图改成黑白风格"},
		},
		Model: "auto",
	}
	chatReq := BuildChatRequest(apiReq)
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected collapsed message, got %d", len(chatReq.Messages))
	}
	parts := chatReq.Messages[0].Content.Parts
	if len(parts) < 2 {
		t.Fatalf("expected image plus transcript text, got %#v", parts)
	}
	imagePart, ok := parts[0].(map[string]interface{})
	if !ok || imagePart["type"] != "input_image" {
		t.Fatalf("expected first collapsed part to be carried input_image, got %#v", parts[0])
	}
	lastPart, ok := parts[len(parts)-1].(string)
	if !ok {
		t.Fatalf("expected transcript text part, got %#v", parts[len(parts)-1])
	}
	if !containsAll(lastPart, "User:\n先生成图片", "Assistant:\n[image omitted from transcript and attached as image input]", "User:\n把刚才那张图改成黑白风格") {
		t.Fatalf("unexpected collapsed transcript: %q", lastPart)
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
