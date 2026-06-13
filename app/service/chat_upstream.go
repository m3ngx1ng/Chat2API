package service

import (
	"bytes"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/types/chat"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/google/uuid"
)

func applyChatTargetDefaults(backend *chatgpt_backend.Client, chatReq *chat.Request) {
	timezone, offset := backend.ChatTimezone()
	chatReq.Timezone = timezone
	chatReq.TimeZoneOffsetMin = offset
}

func shouldUseFConversation(backend *chatgpt_backend.Client) bool {
	return backend.AccAuth != ""
}

func messageHasAssetPointer(message chat.Message) bool {
	if message.Content.ContentType != "multimodal_text" {
		return false
	}
	for _, part := range message.Content.Parts {
		item, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(responseStringValue(item["content_type"], "")) == "image_asset_pointer" {
			return true
		}
		if pointer := strings.TrimSpace(responseStringValue(item["asset_pointer"], "")); strings.HasPrefix(pointer, "file-service://") || strings.HasPrefix(pointer, "sediment://") {
			return true
		}
	}
	return false
}

func applyFConversationPayloadDefaults(chatReq *chat.Request) {
	chatReq.ClientPrepareState = "sent"
	chatReq.EnableMessageFollowups = true
	chatReq.SupportsBuffering = true
	chatReq.SupportedEncodings = []string{"v1"}
	chatReq.HistoryAndTrainingDisabled = false
	chatReq.ParagenCotSummaryDisplayOverride = "allow"
	chatReq.ForceParallelSwitch = "auto"
	chatReq.ThinkingEffort = "standard"
	chatReq.ClientContextualInfo = chat.ClientContextualInfo{
		IsDarkMode:      false,
		TimeSinceLoaded: 1200,
		PageHeight:      1072,
		PageWidth:       1724,
		PixelRatio:      1.2,
		ScreenHeight:    1440,
		ScreenWidth:     2560,
		AppName:         "chatgpt.com",
	}
}

func prepareFConversation(backend *chatgpt_backend.Client, upstreamURL string, chatReq *chat.Request) (string, error) {
	if backend.AccAuth == "" || !strings.HasSuffix(upstreamURL, "/backend-api/f/conversation") {
		return "", nil
	}
	clientPrepareState := strings.TrimSpace(chatReq.ClientPrepareState)
	if clientPrepareState == "" {
		clientPrepareState = "success"
	}
	path := "/backend-api/f/conversation/prepare"
	payload := map[string]interface{}{
		"action":                 "next",
		"fork_from_shared_post":  false,
		"parent_message_id":      chatReq.ParentMessageId,
		"model":                  chatReq.Model,
		"client_prepare_state":   clientPrepareState,
		"timezone_offset_min":    chatReq.TimeZoneOffsetMin,
		"timezone":               chatReq.Timezone,
		"conversation_mode":      chatReq.ConversationMode,
		"system_hints":           chatReq.SystemHints,
		"partial_query":          partialQueryFromChatRequest(chatReq),
		"supports_buffering":     true,
		"supported_encodings":    []string{"v1"},
		"client_contextual_info": map[string]interface{}{"app_name": "chatgpt.com"},
	}
	if thinkingEffort := strings.TrimSpace(chatReq.ThinkingEffort); thinkingEffort != "" {
		payload["thinking_effort"] = thinkingEffort
	}
	if mimeTypes := attachmentMimeTypes(chatReq); len(mimeTypes) > 0 {
		payload["attachment_mime_types"] = mimeTypes
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	headers, cookies := backend.Headers(backend.BaseURL + path)
	headers.Set("accept", "application/json")
	headers.Set("content-type", "application/json")
	applySentinelHeaders(headers, backend, false)
	resp, err := backend.HTTP.Request(tls_client_httpi.POST, backend.BaseURL+path, headers, cookies, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("prepare f conversation failed: %w", err)
	}
	defer resp.Body.Close()
	if !isHTTPSuccess(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("prepare f conversation failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var result struct {
		ConduitToken string `json:"conduit_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.ConduitToken), nil
}

func applySentinelHeaders(headers tls_client_httpi.Headers, backend *chatgpt_backend.Client, includeTurnTrace bool) {
	headers.Set("openai-sentinel-chat-requirements-token", backend.Auth.Token)
	if backend.Auth.ProofWork.Ospt != "" {
		headers.Set("openai-sentinel-proof-token", backend.Auth.ProofWork.Ospt)
	}
	if backend.Auth.TurnstileToken != "" {
		headers.Set("openai-sentinel-turnstile-token", backend.Auth.TurnstileToken)
	}
	if backend.Auth.SoToken != "" {
		headers.Set("openai-sentinel-so-token", backend.Auth.SoToken)
	}
	if includeTurnTrace {
		headers.Set("x-oai-turn-trace-id", uuid.New().String())
	}
}

func partialQueryFromChatRequest(chatReq *chat.Request) map[string]interface{} {
	message := latestUserMessage(chatReq.Messages)
	return map[string]interface{}{
		"id":      uuid.New().String(),
		"author":  map[string]interface{}{"role": "user"},
		"content": message.Content,
	}
}

func latestUserMessage(messages []chat.Message) chat.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Author.Role) == "user" {
			return messages[i]
		}
	}
	if len(messages) > 0 {
		return messages[len(messages)-1]
	}
	return chat.Message{Content: chat.Content{ContentType: "text", Parts: []interface{}{""}}}
}

func attachmentMimeTypes(chatReq *chat.Request) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, message := range chatReq.Messages {
		attachments, ok := message.Metadata["attachments"].([]chat.Attachment)
		if !ok {
			continue
		}
		for _, item := range attachments {
			mimeType := strings.TrimSpace(item.MimeType)
			if mimeType != "" && !seen[mimeType] {
				seen[mimeType] = true
				result = append(result, mimeType)
			}
		}
	}
	return result
}
