package completions

import (
	"strings"

	"chat2api/app/types/chat"

	"github.com/google/uuid"
)

func BuildChatRequest(apiReq *ApiReq) *chat.Request {
	sourceMessages := chatRequestMessages(apiReq)
	messages := make([]chat.Message, 0, len(sourceMessages))
	for _, apiMessage := range sourceMessages {
		content := chatContentFromOpenAI(apiMessage.Content)
		messages = append(messages, chat.Message{
			Id: uuid.New().String(),
			Author: chat.Author{
				Role: apiMessage.Role,
			},
			Content: content,
		})
	}
	parentMessageId := strings.TrimSpace(apiReq.ParentMessageId)
	if parentMessageId == "" {
		parentMessageId = uuid.New().String()
	}

	return &chat.Request{
		Action:                     "next",
		Messages:                   messages,
		ConversationId:             strings.TrimSpace(apiReq.ConversationId),
		ParentMessageId:            parentMessageId,
		Model:                      normalizeModel(apiReq.Model),
		Timezone:                   "Asia/Shanghai",
		TimeZoneOffsetMin:          -480,
		Suggestions:                make([]string, 0),
		SupportedEncodings:         make([]string, 0),
		SystemHints:                make([]string, 0),
		HistoryAndTrainingDisabled: true,
		ForceUseSse:                true,
		FaceUseSse:                 false,
		ForceParagen:               false,
		ForceParagenModelSlug:      "",
		ForceRateLimit:             false,
		ResetRateLimits:            false,
		VariantPurpose:             "comparison_implicit",
		ConversationMode: chat.ConversationMode{
			Kind: "primary_assistant",
		},
		WebsocketRequestId: uuid.New().String(),
		ClientContextualInfo: chat.ClientContextualInfo{
			IsDarkMode:      false,
			TimeSinceLoaded: 120,
			PageHeight:      900,
			PageWidth:       1400,
			PixelRatio:      2,
			ScreenHeight:    1440,
			ScreenWidth:     2560,
		},
	}
}

func chatRequestMessages(apiReq *ApiReq) []ApiMessage {
	if !shouldCollapseStatelessHistory(apiReq) {
		return apiReq.Messages
	}
	systemParts := make([]string, 0)
	transcriptParts := make([]string, 0, len(apiReq.Messages))
	for _, message := range apiReq.Messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(contentToText(message.Content))
		if content == "" {
			continue
		}
		switch role {
		case "system", "developer":
			systemParts = append(systemParts, content)
		default:
			transcriptParts = append(transcriptParts, transcriptRoleLabel(role)+":\n"+content)
		}
	}
	collapsed := make([]ApiMessage, 0, 2)
	if len(systemParts) > 0 {
		collapsed = append(collapsed, ApiMessage{Role: "system", Content: strings.Join(systemParts, "\n\n")})
	}
	if len(transcriptParts) == 0 {
		return apiReq.Messages
	}
	collapsed = append(collapsed, ApiMessage{Role: "user", Content: "Conversation transcript:\n\n" + strings.Join(transcriptParts, "\n\n") + "\n\nRespond to the latest user request directly. Do not repeat prior assistant messages unless the latest user explicitly asks for repetition."})
	return collapsed
}

func shouldCollapseStatelessHistory(apiReq *ApiReq) bool {
	if strings.TrimSpace(apiReq.ConversationId) != "" {
		return false
	}
	assistantCount := 0
	nonSystemCount := 0
	for _, message := range apiReq.Messages {
		role := strings.TrimSpace(message.Role)
		if role != "system" && role != "developer" {
			nonSystemCount++
		}
		if role == "assistant" {
			assistantCount++
		}
	}
	return assistantCount > 0 && nonSystemCount > 1
}

func transcriptRoleLabel(role string) string {
	switch strings.TrimSpace(role) {
	case "assistant":
		return "Assistant"
	case "tool", "function":
		return "Tool"
	default:
		return "User"
	}
}

func chatContentFromOpenAI(content interface{}) chat.Content {
	textParts := make([]string, 0)
	imageParts := make([]interface{}, 0)
	collectOpenAIContent(content, &textParts, &imageParts)
	text := strings.TrimSpace(strings.Join(textParts, ""))
	if len(imageParts) == 0 {
		return chat.Content{ContentType: "text", Parts: []interface{}{text}}
	}
	parts := make([]interface{}, 0, len(imageParts)+1)
	parts = append(parts, imageParts...)
	if text != "" {
		parts = append(parts, text)
	}
	return chat.Content{ContentType: "multimodal_text", Parts: parts}
}

func collectOpenAIContent(value interface{}, textParts *[]string, imageParts *[]interface{}) {
	switch v := value.(type) {
	case string:
		*textParts = append(*textParts, v)
	case []interface{}:
		for _, item := range v {
			collectOpenAIContent(item, textParts, imageParts)
		}
	case map[string]interface{}:
		partType := strings.TrimSpace(stringValue(v["type"]))
		switch partType {
		case "text", "input_text", "output_text":
			*textParts = append(*textParts, stringValue(v["text"]))
		case "image_url", "input_image", "image":
			if image := imageValue(v); image != "" {
				*imageParts = append(*imageParts, map[string]interface{}{"type": "input_image", "image_url": image})
			}
		default:
			if content, ok := v["content"]; ok {
				collectOpenAIContent(content, textParts, imageParts)
			}
		}
	}
}

func stringValue(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func imageValue(item map[string]interface{}) string {
	for _, key := range []string{"image_url", "url", "base64", "b64_json"} {
		value, ok := item[key]
		if !ok {
			continue
		}
		if text := stringValue(value); text != "" {
			return strings.TrimSpace(text)
		}
		if obj, ok := value.(map[string]interface{}); ok {
			for _, nested := range []string{"url", "image_url", "base64", "b64_json"} {
				if text := stringValue(obj[nested]); text != "" {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	if source, ok := item["source"].(map[string]interface{}); ok && stringValue(source["type"]) == "base64" {
		return strings.TrimSpace(stringValue(source["data"]))
	}
	return ""
}

func normalizeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	return model
}
