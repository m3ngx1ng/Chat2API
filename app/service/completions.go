package service

import (
	"bufio"
	"bytes"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/token_pool"
	"chat2api/app/types/chat"
	"chat2api/app/types/completions"
	"chat2api/app/types/responses"
	"chat2api/pkg/logx"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var errToolCallsStreamFinished = errors.New("tool calls stream finished")

func Completions(c *gin.Context) {
	apiReq := &completions.ApiReq{}
	err := c.BindJSON(apiReq)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid parameter", nil)
		return
	}
	if isChatCompletionsImageRequest(apiReq) {
		if err := runChatCompletionsImageRequest(c, apiReq); err != nil {
			logx.WithContext(c.Request.Context()).Error(err.Error())
			common.ErrorResponse(c, http.StatusBadGateway, "image generation failed", err.Error())
		}
		return
	}
	if err := prepareFunctionCallingRequest(apiReq); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	chatReq := completions.BuildChatRequest(apiReq)
	if chatReq.Model == "" {
		errStr := fmt.Sprint("Model is unsupported")
		logx.WithContext(c.Request.Context()).Error(errStr)
		common.ErrorResponse(c, http.StatusBadRequest, errStr, nil)
		return
	}
	upstreamResult, err := sendChatRequestWithBackend(c, chatReq, chatReq.Model)
	if err != nil {
		logx.WithContext(c.Request.Context()).Error(err.Error())
		common.ErrorResponse(c, http.StatusBadGateway, "upstream request failed", err.Error())
		return
	}
	response := upstreamResult.Response
	accessToken := upstreamResult.Token
	defer response.Body.Close()
	if handleResponseError(c, response, accessToken) {
		return
	}
	result, err := handlerResponse(c, apiReq, response, upstreamResult.Backend)
	if err != nil {
		logx.WithContext(c.Request.Context()).Error(err.Error())
		common.ErrorResponse(c, http.StatusBadGateway, "", err.Error())
		return
	}
	if !apiReq.Stream {
		id := completions.GenerateCompletionID(29)
		resp := completions.NewApiRespJson(id, apiReq.Model, result.Content)
		if len(result.ToolCalls) > 0 {
			resp = completions.NewToolCallsApiRespJson(id, apiReq.Model, result.ToolContent, result.ToolCalls)
		}
		resp.ConversationId = result.ConversationId
		resp.MessageId = result.MessageId
		c.JSON(http.StatusOK, resp)
	}
}

func isChatCompletionsImageRequest(apiReq *completions.ApiReq) bool {
	model := strings.ToLower(strings.TrimSpace(apiReq.Model))
	if strings.HasPrefix(model, "gpt-image") || strings.HasPrefix(model, "dall-e") {
		return true
	}
	for _, tool := range apiReq.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Type), "image_generation") {
			return true
		}
	}
	return false
}

func runChatCompletionsImageRequest(c *gin.Context, apiReq *completions.ApiReq) error {
	prompt, images := chatCompletionsImagePrompt(apiReq.Messages)
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("image generation requires user prompt")
	}
	tool := normalizeCodexImageTool(responsesImageToolFromCompletions(apiReq), len(images) > 0)
	if strings.TrimSpace(tool.Model) == "" {
		tool.Model = strings.TrimSpace(apiReq.Model)
	}
	completed, err := collectConversationImageResponse(c, prompt, images, tool)
	if err != nil {
		return err
	}
	b64, revised := imageResultFromCompleted(completed)
	if b64 == "" {
		return fmt.Errorf("upstream completed without generating images")
	}
	return writeChatCompletionsImageResponse(c, apiReq, b64, revised, completed)
}

func chatCompletionsImagePrompt(messages []completions.ApiMessage) (string, []string) {
	parts := make([]string, 0)
	images := make([]string, 0)
	for _, message := range messages {
		if strings.TrimSpace(message.Role) != "user" {
			continue
		}
		collectChatCompletionImageContent(message.Content, &parts, &images)
	}
	return strings.TrimSpace(strings.Join(parts, "")), images
}

func collectChatCompletionImageContent(value interface{}, textParts *[]string, images *[]string) {
	switch v := value.(type) {
	case string:
		*textParts = append(*textParts, v)
	case []interface{}:
		for _, item := range v {
			collectChatCompletionImageContent(item, textParts, images)
		}
	case map[string]interface{}:
		partType := strings.TrimSpace(responseStringValue(v["type"], ""))
		switch partType {
		case "text", "input_text", "output_text":
			if text := responseStringValue(v["text"], ""); text != "" {
				*textParts = append(*textParts, text)
			}
		case "image_url", "input_image", "image":
			if image := chatCompletionImageValue(v); image != "" {
				*images = append(*images, image)
			}
		default:
			if content, ok := v["content"]; ok {
				collectChatCompletionImageContent(content, textParts, images)
			}
		}
	}
}

func chatCompletionImageValue(item map[string]interface{}) string {
	for _, key := range []string{"image_url", "url", "base64", "b64_json"} {
		value, ok := item[key]
		if !ok {
			continue
		}
		if text := responseStringValue(value, ""); text != "" {
			return strings.TrimSpace(text)
		}
		if obj, ok := value.(map[string]interface{}); ok {
			for _, nested := range []string{"url", "image_url", "base64", "b64_json"} {
				if text := responseStringValue(obj[nested], ""); text != "" {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	if source, ok := item["source"].(map[string]interface{}); ok && responseStringValue(source["type"], "") == "base64" {
		return strings.TrimSpace(responseStringValue(source["data"], ""))
	}
	return ""
}

func responsesImageToolFromCompletions(apiReq *completions.ApiReq) responses.Tool {
	for _, tool := range apiReq.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Type), "image_generation") {
			return responses.Tool{Type: "image_generation", Model: strings.TrimSpace(apiReq.Model)}
		}
	}
	return responses.Tool{Type: "image_generation", Model: strings.TrimSpace(apiReq.Model)}
}

func writeChatCompletionsImageResponse(c *gin.Context, apiReq *completions.ApiReq, b64 string, revised string, completed map[string]interface{}) error {
	if strings.TrimSpace(revised) == "" {
		revised = strings.TrimSpace(chatCompletionsPromptFallback(apiReq.Messages))
	}
	content := "![image](data:image/png;base64," + b64 + ")"
	if apiReq.Stream {
		id := completions.GenerateCompletionID(29)
		created := completions.NewApiRespStream(id, apiReq.Model, content)
		created.Choices[0].Delta.Role = "assistant"
		if _, err := c.Writer.WriteString("data: " + created.String() + "\n\n"); err != nil {
			return err
		}
		finalLine := completions.StopChunk(id, apiReq.Model, "stop")
		if _, err := c.Writer.WriteString(fmt.Sprint("data: ", finalLine.String(), "\n\n")); err != nil {
			return err
		}
		if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	id := completions.GenerateCompletionID(29)
	resp := completions.NewApiRespJson(id, apiReq.Model, content)
	if conversationID := strings.TrimSpace(responseStringValue(completed["conversation_id"], "")); conversationID != "" {
		resp.ConversationId = conversationID
	}
	c.JSON(http.StatusOK, resp)
	return nil
}

func chatCompletionsPromptFallback(messages []completions.ApiMessage) string {
	prompt, _ := chatCompletionsImagePrompt(messages)
	return prompt
}

func prepareFunctionCallingRequest(apiReq *completions.ApiReq) error {
	completions.NormalizeLegacyFunctions(apiReq)
	hasTools := completions.HasTools(apiReq)
	apiReq.HasToolResults = completions.MessagesContainToolResults(apiReq.Messages)
	if completions.MessagesNeedPreprocess(apiReq.Messages) {
		processed, err := completions.PreprocessMessages(apiReq.Messages)
		if err != nil {
			return err
		}
		apiReq.Messages = processed
	}
	if !hasTools {
		return nil
	}
	prompt, err := completions.BuildFunctionPrompt(apiReq.Tools, apiReq.ToolChoice)
	if err != nil {
		return err
	}
	apiReq.Messages = append([]completions.ApiMessage{{Role: "system", Content: prompt}}, apiReq.Messages...)
	return nil
}

func handleResponseError(c *gin.Context, response *http.Response, accessToken string) bool {
	if response.StatusCode == http.StatusOK {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if response.StatusCode == http.StatusTooManyRequests {
		canUseAt := rateLimitCanUseAt(response, body)
		token_pool.GetAccessTokenPool().SetCanUseAt(accessToken, canUseAt)
	}
	var errorResponse map[string]interface{}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&errorResponse); err != nil {
		common.ErrorResponse(c, response.StatusCode, "Unknown error", errors.New(string(body)))
		return true
	}
	common.ErrorResponse(c, response.StatusCode, errorResponse["detail"], nil)
	return true
}

func rateLimitCanUseAt(response *http.Response, body []byte) int64 {
	now := time.Now()
	if value := parseRetryAfter(response.Header.Get("Retry-After"), now); value > 0 {
		return value
	}
	if value := parseRateLimitBody(body, now); value > 0 {
		return value
	}
	return now.Add(time.Hour).Unix()
}

func parseRetryAfter(value string, now time.Time) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		return now.Add(time.Duration(seconds) * time.Second).Unix()
	}
	if t, err := http.ParseTime(value); err == nil {
		return t.Unix()
	}
	return 0
}

func parseRateLimitBody(body []byte, now time.Time) int64 {
	if len(body) == 0 {
		return 0
	}
	var payload interface{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return 0
	}
	return findRateLimitTime(payload, now)
}

func findRateLimitTime(value interface{}, now time.Time) int64 {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"retry_after", "reset_after", "resets_after", "restore_at", "reset_at"} {
			if candidate, ok := v[key]; ok {
				if parsed := parseRateLimitValue(candidate, now); parsed > 0 {
					return parsed
				}
			}
		}
		for _, child := range v {
			if parsed := findRateLimitTime(child, now); parsed > 0 {
				return parsed
			}
		}
	case []interface{}:
		for _, child := range v {
			if parsed := findRateLimitTime(child, now); parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func parseRateLimitValue(value interface{}, now time.Time) int64 {
	switch v := value.(type) {
	case json.Number:
		if seconds, err := v.Int64(); err == nil {
			return normalizeRateLimitUnix(seconds, now)
		}
		if f, err := v.Float64(); err == nil {
			return normalizeRateLimitUnix(int64(f), now)
		}
	case float64:
		return normalizeRateLimitUnix(int64(v), now)
	case string:
		return parseRateLimitString(v, now)
	}
	return 0
}

func normalizeRateLimitUnix(value int64, now time.Time) int64 {
	if value <= 0 {
		return 0
	}
	if value < 30*24*3600 {
		return now.Add(time.Duration(value) * time.Second).Unix()
	}
	return value
}

func parseRateLimitString(value string, now time.Time) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return normalizeRateLimitUnix(seconds, now)
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return now.Add(duration).Unix()
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, time.DateTime, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Unix()
		}
	}
	if t, err := http.ParseTime(value); err == nil {
		return t.Unix()
	}
	return 0
}

type chatResult struct {
	Content        string
	ConversationId string
	MessageId      string
	FinishReason   string
	ToolCalls      []completions.ToolCall
	ToolContent    string
	ImageTask      bool
	ImageFileID    string
}

type chatStreamEvent struct {
	Response     chat.Response
	Delta        string
	Text         string
	IsFirstChunk bool
	Result       *chatResult
}

func handleChatStream(resp *http.Response, onEvent func(chatStreamEvent) error) (*chatResult, error) {
	reader := bufio.NewReader(resp.Body)
	var previousText chat.StringStruct
	isFirstChunk := true
	result := &chatResult{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var rawEvent map[string]interface{}
		_ = json.Unmarshal([]byte(payload), &rawEvent)
		var chatResp chat.Response
		if err := json.Unmarshal([]byte(payload), &chatResp); err != nil {
			continue
		}
		if chatResp.Error != nil {
			return nil, fmt.Errorf("chatgpt error: %v", chatResp.Error)
		}
		if chatResp.ConversationId != "" {
			result.ConversationId = chatResp.ConversationId
		}
		if chatResp.Message.Id != "" {
			result.MessageId = chatResp.Message.Id
		}
		if imageTask := chatImageTaskFromRawEvent(rawEvent); imageTask || result.ImageFileID == "" {
			if imageTask {
				result.ImageTask = true
			}
			if fileID := findImageFileID(rawEvent); fileID != "" {
				result.ImageTask = true
				result.ImageFileID = fileID
			}
		}
		if chatResp.Message.Metadata.MessageType != "" &&
			chatResp.Message.Metadata.MessageType != "next" &&
			chatResp.Message.Metadata.MessageType != "continue" {
			continue
		}
		text := chatResponseText(chatResp)
		if text == "" {
			text = assistantRawText(rawEvent, previousText.Text)
			if text != "" && chatResp.Message.Author.Role == "" {
				chatResp.Message.Author.Role = "assistant"
			}
		}
		if text == "" {
			continue
		}
		delta := completions.DeltaText(text, previousText.Text)
		if !isFirstChunk && delta == "" {
			continue
		}
		previousText.Text = text
		if onEvent != nil {
			if err := onEvent(chatStreamEvent{
				Response:     chatResp,
				Delta:        delta,
				Text:         previousText.Text,
				IsFirstChunk: isFirstChunk,
				Result:       result,
			}); err != nil {
				result.Content = previousText.Text
				return result, err
			}
		}
		isFirstChunk = false
		if chatResp.Message.Metadata.FinishDetails != nil {
			result.FinishReason = chatResp.Message.Metadata.FinishDetails.Type
		}
	}
	result.Content = previousText.Text
	return result, nil
}

func chatImageTaskFromRawEvent(event map[string]interface{}) bool {
	if len(event) == 0 {
		return false
	}
	if strings.Contains(strings.ToLower(responseStringValue(event["type"], "")), "image") {
		return true
	}
	if chatImageTaskFromServerMetadata(responseMapValue(event["metadata"])) {
		return true
	}
	if chatImageTaskFromMap(responseMapValue(event["message"])) {
		return true
	}
	vMap := responseMapValue(event["v"])
	if chatImageTaskFromMap(responseMapValue(vMap["message"])) || chatImageTaskFromMap(vMap) {
		return true
	}
	return chatImageTaskFromServerMetadata(responseMapValue(vMap["metadata"]))
}

func chatImageTaskFromMap(value map[string]interface{}) bool {
	if len(value) == 0 {
		return false
	}
	metadata := responseMapValue(value["metadata"])
	if len(metadata) == 0 {
		return false
	}
	if responseStringValue(metadata["image_gen_task_id"], "") != "" {
		return true
	}
	if v, ok := metadata["image_gen_multi_stream"].(bool); ok && v {
		return true
	}
	if v, ok := metadata["ui_card"].(bool); ok && v && strings.Contains(strings.ToLower(responseStringValue(metadata["ui_card_title"], "")), "图") {
		return true
	}
	return false
}

func chatImageTaskFromServerMetadata(metadata map[string]interface{}) bool {
	if len(metadata) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(responseStringValue(metadata["turn_use_case"], "")), "image gen") {
		return true
	}
	if invoked, ok := metadata["tool_invoked"].(bool); ok && invoked {
		toolName := strings.TrimSpace(responseStringValue(metadata["tool_name"], ""))
		if strings.Contains(strings.ToLower(toolName), "image") {
			return true
		}
	}
	return false
}

func chatResponseText(chatResp chat.Response) string {
	if chatResp.Message.Author.Role != "assistant" {
		return ""
	}
	if text := strings.TrimSpace(chatResp.Message.Content.Text); text != "" {
		return text
	}
	if chatResp.Message.Content.ContentType != "" && !strings.Contains(chatResp.Message.Content.ContentType, "text") {
		return ""
	}
	parts := make([]string, 0)
	for _, part := range chatResp.Message.Content.Parts {
		switch v := part.(type) {
		case string:
			parts = append(parts, v)
		case map[string]interface{}:
			if text := strings.TrimSpace(responseStringValue(v["text"], "")); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func assistantRawText(event map[string]interface{}, currentText string) string {
	if len(event) == 0 {
		return ""
	}
	if text := assistantTextFromMessageMap(responseMapValue(event["message"])); text != "" {
		return text
	}
	vMap := responseMapValue(event["v"])
	if text := assistantTextFromMessageMap(responseMapValue(vMap["message"])); text != "" {
		return text
	}
	if text, ok := applyAssistantTextPatch(event, currentText); ok {
		return text
	}
	return ""
}

func assistantTextFromMessageMap(message map[string]interface{}) string {
	if len(message) == 0 {
		return ""
	}
	if author := responseMapValue(message["author"]); len(author) > 0 {
		if role := strings.TrimSpace(responseStringValue(author["role"], "")); role != "" && role != "assistant" {
			return ""
		}
	}
	content := responseMapValue(message["content"])
	if len(content) == 0 {
		return ""
	}
	if text := strings.TrimSpace(responseStringValue(content["text"], "")); text != "" {
		return text
	}
	contentType := strings.TrimSpace(responseStringValue(content["content_type"], ""))
	if contentType != "" && !strings.Contains(contentType, "text") {
		return ""
	}
	return strings.TrimSpace(textFromContentParts(content["parts"]))
}

func textFromContentParts(value interface{}) string {
	parts, ok := value.([]interface{})
	if !ok {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch v := part.(type) {
		case string:
			texts = append(texts, v)
		case map[string]interface{}:
			if text := responseStringValue(v["text"], ""); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "")
}

func applyAssistantTextPatch(event map[string]interface{}, currentText string) (string, bool) {
	path := responseStringValue(event["p"], responseStringValue(event["path"], ""))
	if path != "" && !isAssistantTextPath(path) && !strings.HasPrefix(path, "/message/content/parts/0/") {
		return "", false
	}
	op := responseStringValue(event["o"], responseStringValue(event["op"], ""))
	if op == "patch" {
		return applyAssistantTextPatchOps(event["v"], currentText)
	}
	if op == "append" || op == "add" {
		return currentText + patchTextValue(event["v"]), true
	}
	if op == "replace" {
		return patchTextValue(event["v"]), true
	}
	if value, ok := event["v"].(string); ok && value != "" && isAssistantTextPath(path) {
		return currentText + value, true
	}
	return "", false
}

func applyAssistantTextPatchOps(value interface{}, currentText string) (string, bool) {
	ops, ok := value.([]interface{})
	if !ok {
		return "", false
	}
	text := currentText
	applied := false
	for _, item := range ops {
		opMap := responseMapValue(item)
		if len(opMap) == 0 {
			continue
		}
		path := responseStringValue(opMap["p"], responseStringValue(opMap["path"], ""))
		if path != "" && !isAssistantTextPath(path) && !strings.HasPrefix(path, "/message/content/parts/0/") {
			continue
		}
		op := responseStringValue(opMap["o"], responseStringValue(opMap["op"], ""))
		switch op {
		case "patch":
			next, ok := applyAssistantTextPatchOps(opMap["v"], text)
			if ok {
				text = next
				applied = true
			}
		case "append", "add":
			text += patchTextValue(opMap["v"])
			applied = true
		case "replace":
			text = patchTextValue(opMap["v"])
			applied = true
		}
	}
	return text, applied
}

func isAssistantTextPath(path string) bool {
	return path == "" || path == "/message/content/parts/0" || path == "/message/content/text"
}

func patchTextValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text := responseStringValue(v["text"], ""); text != "" {
			return text
		}
		return textFromContentParts(v["parts"])
	case []interface{}:
		return textFromContentParts(v)
	default:
		return ""
	}
}

func responseMapValue(value interface{}) map[string]interface{} {
	if v, ok := value.(map[string]interface{}); ok {
		return v
	}
	return nil
}

func handlerResponse(c *gin.Context, apiReq *completions.ApiReq, resp *http.Response, backend *chatgpt_backend.Client) (*chatResult, error) {
	if apiReq.Stream {
		c.Header("Content-Type", "text/event-stream")
	} else {
		c.Header("Content-Type", "application/json")
	}
	id := completions.GenerateCompletionID(29)
	hasTools := completions.HasTools(apiReq)
	detector := completions.NewStreamToolDetector(completions.ToolifyTriggerSignal)
	result, err := handleChatStream(resp, func(event chatStreamEvent) error {
		if !apiReq.Stream {
			return nil
		}
		if event.Result != nil && event.Result.ImageTask {
			return nil
		}
		if hasTools {
			return streamFunctionCallingDelta(c, id, apiReq, detector, event)
		}
		apiRespJson := completions.NewApiRespStream(id, apiReq.Model, event.Delta)
		apiRespJson.ConversationId = event.Response.ConversationId
		apiRespJson.MessageId = event.Response.Message.Id
		if event.IsFirstChunk {
			apiRespJson.Choices[0].Delta.Role = event.Response.Message.Author.Role
		}
		if _, err := c.Writer.WriteString("data: " + apiRespJson.String() + "\n\n"); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	})
	if err != nil && err != errToolCallsStreamFinished {
		return nil, err
	}
	if result == nil {
		result = &chatResult{}
	}
	if hasTools && len(result.ToolCalls) == 0 {
		if calls := completions.ParseFunctionCallsXML(result.Content, completions.ToolifyTriggerSignal); len(calls) > 0 {
			if err := completions.ValidateParsedToolCalls(calls, apiReq.Tools); err == nil {
				result.ToolCalls = completions.ToolCallsFromParsed(calls, false)
				result.ToolContent = completions.ToolCallPrefixText(result.Content)
				result.FinishReason = "tool_calls"
			}
		}
	}
	if !hasTools && apiReq.HasToolResults {
		result.Content = completions.StripFunctionCallXML(result.Content)
	}
	if result.ImageTask && result.ConversationId != "" && backend != nil {
		completed, err := chatCompletionImageCompletedFromResult(backend, result)
		if err != nil {
			return nil, err
		}
		if b64, _ := imageResultFromCompleted(completed); b64 != "" {
			result.Content = "![image](data:image/png;base64," + b64 + ")"
			result.FinishReason = "stop"
			if apiReq.Stream {
				id := completions.GenerateCompletionID(29)
				chunk := completions.NewApiRespStream(id, apiReq.Model, result.Content)
				chunk.ConversationId = result.ConversationId
				chunk.MessageId = result.MessageId
				chunk.Choices[0].Delta.Role = "assistant"
				if _, err := c.Writer.WriteString("data: " + chunk.String() + "\n\n"); err != nil {
					return nil, err
				}
				finalLine := completions.StopChunk(id, apiReq.Model, "stop")
				if _, err := c.Writer.WriteString(fmt.Sprint("data: ", finalLine.String(), "\n\n")); err != nil {
					return nil, err
				}
				if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err != nil {
					return nil, err
				}
				c.Writer.Flush()
			}
			return result, nil
		}
	}
	if apiReq.Stream && hasTools {
		if err == errToolCallsStreamFinished {
			return result, nil
		}
		if detector.State() == "tool_parsing" {
			if calls := detector.Finalize(); len(calls) > 0 {
				if err := completions.ValidateParsedToolCalls(calls, apiReq.Tools); err == nil {
					result.ToolCalls = completions.ToolCallsFromParsed(calls, true)
					result.ToolContent = completions.ToolCallPrefixText(result.Content)
					if writeErr := writeToolCallsStream(c, id, apiReq.Model, result.ToolCalls); writeErr != nil {
						return nil, writeErr
					}
					return result, nil
				}
			}
			if detector.Buffer() != "" {
				apiRespJson := completions.NewApiRespStream(id, apiReq.Model, detector.Buffer())
				if _, err := c.Writer.WriteString("data: " + apiRespJson.String() + "\n\n"); err != nil {
					return nil, err
				}
			}
		} else if text := detector.FlushText(); text != "" {
			apiRespJson := completions.NewApiRespStream(id, apiReq.Model, text)
			if _, err := c.Writer.WriteString("data: " + apiRespJson.String() + "\n\n"); err != nil {
				return nil, err
			}
		}
	}
	if apiReq.Stream {
		finalLine := completions.StopChunk(id, apiReq.Model, result.FinishReason)
		_, _ = c.Writer.WriteString(fmt.Sprint("data: ", finalLine.String(), "\n\n"))
		_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	}
	return result, nil
}

func chatCompletionImageCompletedFromResult(backend *chatgpt_backend.Client, result *chatResult) (map[string]interface{}, error) {
	if result.ImageFileID != "" {
		if b64, revised, err := downloadConversationImageFile(backend, result.ConversationId, result.ImageFileID); err == nil && b64 != "" {
			return conversationImageCompleted(b64, revised), nil
		}
	}
	return pollConversationImageResult(backend, result.ConversationId)
}

func streamFunctionCallingDelta(c *gin.Context, id string, apiReq *completions.ApiReq, detector *completions.StreamToolDetector, event chatStreamEvent) error {
	if detector.State() == "tool_parsing" {
		detector.AppendParsing(event.Delta)
		if !detector.HasCompleteToolBlock() {
			return nil
		}
		calls := detector.Finalize()
		if len(calls) == 0 || completions.ValidateParsedToolCalls(calls, apiReq.Tools) != nil {
			finalLine := completions.StopChunk(id, apiReq.Model, "stop")
			if _, err := c.Writer.WriteString(fmt.Sprint("data: ", finalLine.String(), "\n\n")); err != nil {
				return err
			}
			if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err != nil {
				return err
			}
			c.Writer.Flush()
			return errToolCallsStreamFinished
		}
		event.Result.ToolCalls = completions.ToolCallsFromParsed(calls, true)
		event.Result.ToolContent = completions.ToolCallPrefixText(event.Text)
		event.Result.FinishReason = "tool_calls"
		if err := writeToolCallsStream(c, id, apiReq.Model, event.Result.ToolCalls); err != nil {
			return err
		}
		return errToolCallsStreamFinished
	}

	detected, content := detector.ProcessChunk(event.Delta)
	if content != "" {
		apiRespJson := completions.NewApiRespStream(id, apiReq.Model, content)
		apiRespJson.ConversationId = event.Response.ConversationId
		apiRespJson.MessageId = event.Response.Message.Id
		if event.IsFirstChunk {
			apiRespJson.Choices[0].Delta.Role = event.Response.Message.Author.Role
		}
		if _, err := c.Writer.WriteString("data: " + apiRespJson.String() + "\n\n"); err != nil {
			return err
		}
		c.Writer.Flush()
	}
	if detected {
		return nil
	}
	return nil
}

func writeToolCallsStream(c *gin.Context, id string, model string, toolCalls []completions.ToolCall) error {
	toolChunk := completions.NewToolCallsApiRespStream(id, model, toolCalls)
	if _, err := c.Writer.WriteString("data: " + toolChunk.String() + "\n\n"); err != nil {
		return err
	}
	finalLine := completions.StopChunk(id, model, "tool_calls")
	if _, err := c.Writer.WriteString(fmt.Sprint("data: ", finalLine.String(), "\n\n")); err != nil {
		return err
	}
	if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
