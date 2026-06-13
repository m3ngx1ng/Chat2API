package service

import (
	"bufio"
	"bytes"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/types/responses"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/gin-gonic/gin"
)

const codexResponsesModel = "gpt-5.5"
const codexResponsesInstructions = "You are an image generation bridge. You must call the image_generation tool exactly once for the user's request. Do not answer with text. Do not claim that image generation is unavailable, disabled, or unsupported. Return only the generated image result."

type codexResponsesPayload struct {
	Model             string                   `json:"model"`
	Instructions      string                   `json:"instructions"`
	Store             bool                     `json:"store"`
	Input             []map[string]interface{} `json:"input"`
	Tools             []responses.Tool         `json:"tools"`
	ToolChoice        map[string]string        `json:"tool_choice"`
	Stream            bool                     `json:"stream"`
	ParallelToolCalls bool                     `json:"parallel_tool_calls"`
}

func runCodexImageResponses(c *gin.Context, apiReq *responses.ApiReq) error {
	currentInput := latestResponsesUserInput(apiReq.Input)
	prompt := extractResponsesPrompt(currentInput)
	if strings.TrimSpace(prompt) == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "input text is required", nil)
		return nil
	}
	images := extractResponsesImages(currentInput)
	tool := normalizeCodexImageTool(firstResponsesImageGenerationTool(apiReq.Tools), len(images) > 0)
	return runConversationImageResponses(c, apiReq, prompt, images, tool)
}

func runConversationImageResponses(c *gin.Context, apiReq *responses.ApiReq, prompt string, images []string, tool responses.Tool) error {
	completed, err := collectConversationImageResponse(c, prompt, images, tool)
	if err != nil {
		return err
	}
	output := responsesOutputFromCompleted(completed, prompt)
	responseID := responses.ResponseID()
	created := time.Now().Unix()
	model := responses.NormalizeModel(apiReq.Model)
	if apiReq.Stream {
		c.Header("Content-Type", "text/event-stream")
		createdEvent := responses.CreatedEvent(responseID, model, created)
		if _, err := c.Writer.WriteString(responses.SSE(createdEvent)); err != nil {
			return err
		}
		for index, item := range output {
			addedItem := item
			if _, err := c.Writer.WriteString(responses.SSE(responses.Event{Type: "response.output_item.added", OutputIndex: index, Item: &addedItem})); err != nil {
				return err
			}
			if _, err := c.Writer.WriteString(responses.SSE(responses.Event{Type: "response.output_item.done", OutputIndex: index, Item: &addedItem})); err != nil {
				return err
			}
		}
		if _, err := c.Writer.WriteString(responses.SSE(responses.CompletedEvent(responseID, model, created, output))); err != nil {
			return err
		}
		_, _ = c.Writer.WriteString("data: [DONE]\n\n")
		c.Writer.Flush()
		return nil
	}
	c.JSON(http.StatusOK, responses.CompletedEvent(responseID, model, created, output).Response)
	return nil
}

func responsesOutputFromCompleted(completed map[string]interface{}, prompt string) []responses.OutputItem {
	output := make([]responses.OutputItem, 0)
	rawOutput, _ := completed["output"].([]interface{})
	for _, raw := range rawOutput {
		itemMap := responseMapValue(raw)
		if b64, revised := imageResultFromCompleted(itemMap); b64 != "" {
			if revised == "" {
				revised = prompt
			}
			output = append(output, responses.ImageOutputItem(responses.MessageID(), b64, revised))
		}
	}
	if len(output) == 0 {
		if b64, revised := imageResultFromCompleted(completed); b64 != "" {
			if revised == "" {
				revised = prompt
			}
			output = append(output, responses.ImageOutputItem(responses.MessageID(), b64, revised))
		}
	}
	return output
}

func sendCodexResponsesRequest(c *gin.Context, payload codexResponsesPayload) (*http.Response, string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	result, err := executeWithModelCandidates(c, payload.Model, func(backend *chatgpt_backend.Client) (*http.Response, error) {
		url := backend.BaseURL + "/backend-api/codex/responses"
		headers, cookies := backend.Headers(url)
		headers.Set("content-type", "application/json")
		if backend.AccAuth == "" {
			return nil, fmt.Errorf("codex responses endpoint requires access token auth")
		}
		resp, err := backend.HTTP.Request(tls_client_httpi.POST, url, headers, cookies, bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("upstream codex responses request failed: %w", err)
		}
		return resp, nil
	})
	if err != nil {
		return nil, "", err
	}
	return result.Response, result.Token, nil
}

func completedHasImage(completed map[string]interface{}) bool {
	if completed == nil {
		return false
	}
	b64, _ := imageResultFromCompleted(completed)
	return b64 != ""
}

func streamCompletedImageResponse(c *gin.Context, apiReq *responses.ApiReq, completed map[string]interface{}, prompt string) error {
	output := responsesOutputFromCompleted(completed, prompt)
	responseID := responses.ResponseID()
	created := time.Now().Unix()
	model := responses.NormalizeModel(apiReq.Model)
	c.Header("Content-Type", "text/event-stream")
	if _, err := c.Writer.WriteString(responses.SSE(responses.CreatedEvent(responseID, model, created))); err != nil {
		return err
	}
	for index, item := range output {
		itemCopy := item
		if _, err := c.Writer.WriteString(responses.SSE(responses.Event{Type: "response.output_item.added", OutputIndex: index, Item: &itemCopy})); err != nil {
			return err
		}
		if _, err := c.Writer.WriteString(responses.SSE(responses.Event{Type: "response.output_item.done", OutputIndex: index, Item: &itemCopy})); err != nil {
			return err
		}
	}
	if _, err := c.Writer.WriteString(responses.SSE(responses.CompletedEvent(responseID, model, created, output))); err != nil {
		return err
	}
	_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
	return nil
}

func streamCodexResponses(c *gin.Context, resp *http.Response) error {
	c.Header("Content-Type", "text/event-stream")
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "data: [DONE]" {
			if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err != nil {
				return err
			}
			break
		}
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if !isPublicCodexResponseEvent(payload) {
			continue
		}
		if _, err := c.Writer.WriteString("data: " + payload + "\n\n"); err != nil {
			return err
		}
		c.Writer.Flush()
	}
	c.Writer.Flush()
	return nil
}

func isPublicCodexResponseEvent(payload string) bool {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return false
	}
	switch responseStringValue(event["type"], "") {
	case "response.created",
		"response.output_item.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.output_item.done",
		"response.completed",
		"response.failed",
		"response.incomplete":
		return true
	default:
		return false
	}
}

func collectCodexResponse(body io.Reader) (map[string]interface{}, error) {
	reader := bufio.NewReader(body)
	var completed map[string]interface{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if event["type"] == "response.completed" {
			if response, ok := event["response"].(map[string]interface{}); ok {
				completed = response
			}
		}
	}
	if completed == nil {
		return nil, fmt.Errorf("codex response generation failed")
	}
	if image, _ := imageResultFromCompleted(completed); image == "" {
		if text := strings.TrimSpace(codexResponseText(completed)); text != "" {
			return nil, fmt.Errorf("codex response completed without image: %s", text)
		}
	}
	return completed, nil
}

func latestResponsesUserInput(input interface{}) interface{} {
	items, ok := input.([]interface{})
	if !ok || len(items) == 0 {
		return input
	}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(map[string]interface{})
		if !ok {
			continue
		}
		role := strings.TrimSpace(responseStringValue(item["role"], ""))
		if role == "user" {
			return item
		}
	}
	return items[len(items)-1]
}

func codexResponseText(value interface{}) string {
	parts := make([]string, 0)
	collectCodexResponseText(value, &parts)
	return strings.TrimSpace(strings.Join(parts, ""))
}

func collectCodexResponseText(value interface{}, parts *[]string) {
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			collectCodexResponseText(item, parts)
		}
	case map[string]interface{}:
		if responseStringValue(v["type"], "") == "output_text" {
			if text := responseStringValue(v["text"], ""); text != "" {
				*parts = append(*parts, text)
			}
		}
		for _, child := range v {
			collectCodexResponseText(child, parts)
		}
	}
}

func firstResponsesImageGenerationTool(tools []responses.Tool) responses.Tool {
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) == "image_generation" {
			return tool
		}
	}
	return responses.Tool{Type: "image_generation"}
}

func normalizeCodexImageTool(tool responses.Tool, hasImages bool) responses.Tool {
	tool.Type = "image_generation"
	if strings.TrimSpace(tool.Model) == "" {
		tool.Model = "gpt-image-2"
	}
	if strings.TrimSpace(tool.Action) == "" {
		if hasImages {
			tool.Action = "edit"
		} else {
			tool.Action = "generate"
		}
	}
	if strings.TrimSpace(tool.Size) == "" {
		tool.Size = "1024x1024"
	}
	if strings.TrimSpace(tool.Quality) == "" {
		tool.Quality = "auto"
	}
	if strings.TrimSpace(tool.OutputFormat) == "" {
		tool.OutputFormat = "png"
	}
	return tool
}

func codexImageInput(prompt string, images []string) []map[string]interface{} {
	content := []map[string]interface{}{{"type": "input_text", "text": prompt}}
	for _, image := range images {
		content = append(content, map[string]interface{}{"type": "input_image", "image_url": normalizeImageDataURL(image)})
	}
	return []map[string]interface{}{{"role": "user", "content": content}}
}

func normalizeImageDataURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "data:image/") {
		return value
	}
	if value != "" && len(value) < 512 && !strings.ContainsAny(value, "\r\n") {
		if data, err := os.ReadFile(filepath.Clean(value)); err == nil && len(data) > 0 {
			return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
	return "data:image/png;base64," + value
}

func extractResponsesPrompt(input interface{}) string {
	parts := make([]string, 0)
	collectResponsesText(input, &parts)
	return strings.TrimSpace(strings.Join(parts, ""))
}

func collectResponsesText(value interface{}, parts *[]string) {
	switch v := value.(type) {
	case string:
		*parts = append(*parts, v)
	case []interface{}:
		for _, item := range v {
			collectResponsesText(item, parts)
		}
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok && text != "" {
			*parts = append(*parts, text)
			return
		}
		if content, ok := v["content"]; ok {
			collectResponsesText(content, parts)
		}
	}
}

func extractResponsesImages(input interface{}) []string {
	images := make([]string, 0)
	collectResponsesImages(input, &images)
	return images
}

func collectResponsesImages(value interface{}, images *[]string) {
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			collectResponsesImages(item, images)
		}
	case map[string]interface{}:
		partType := strings.TrimSpace(responseStringValue(v["type"], ""))
		if partType == "input_image" || partType == "image" || partType == "image_url" || v["image_url"] != nil {
			if image := responseImageValue(v); image != "" {
				*images = append(*images, image)
			}
		}
		if content, ok := v["content"]; ok {
			collectResponsesImages(content, images)
		}
	}
}

func responseImageValue(item map[string]interface{}) string {
	for _, key := range []string{"image_url", "url", "base64", "b64_json"} {
		value, ok := item[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
		if obj, ok := value.(map[string]interface{}); ok {
			for _, nested := range []string{"url", "image_url", "base64", "b64_json"} {
				if text, ok := obj[nested].(string); ok {
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
