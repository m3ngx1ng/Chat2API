package service

import (
	"bufio"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/types/chat"
	"chat2api/app/types/completions"
	"chat2api/app/types/responses"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

type ImagesGenerationsRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	Quality        string `json:"quality"`
	ResponseFormat string `json:"response_format"`
}

type ImagesEditsRequest struct {
	Model          string      `json:"model"`
	Prompt         string      `json:"prompt"`
	N              int         `json:"n"`
	Size           string      `json:"size"`
	Quality        string      `json:"quality"`
	ResponseFormat string      `json:"response_format"`
	Image          interface{} `json:"image"`
	Images         interface{} `json:"images"`
	ImageURL       interface{} `json:"image_url"`
}

type ImagesResponse struct {
	Created int64            `json:"created"`
	Data    []ImagesRespItem `json:"data"`
}

type ImagesRespItem struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func ImagesGenerations(c *gin.Context) {
	req := &ImagesGenerationsRequest{}
	if err := c.BindJSON(req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid parameter", nil)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "prompt is required", nil)
		return
	}
	result, err := runOpenAIImages(c, req.Prompt, nil, imageOptions{
		Model:          req.Model,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		ResponseFormat: req.ResponseFormat,
	})
	if err != nil {
		common.ErrorResponse(c, http.StatusBadGateway, "image generation failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func ImagesEdits(c *gin.Context) {
	if strings.HasPrefix(strings.ToLower(c.ContentType()), "multipart/form-data") {
		handleMultipartImagesEdits(c)
		return
	}
	req := &ImagesEditsRequest{}
	if err := c.BindJSON(req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid parameter", nil)
		return
	}
	images := imageValuesFromJSON(req.Image, req.Images, req.ImageURL)
	if len(images) == 0 {
		common.ErrorResponse(c, http.StatusBadRequest, "image is required", nil)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "prompt is required", nil)
		return
	}
	result, err := runOpenAIImages(c, req.Prompt, images, imageOptions{
		Model:          req.Model,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		ResponseFormat: req.ResponseFormat,
	})
	if err != nil {
		common.ErrorResponse(c, http.StatusBadGateway, "image edit failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func handleMultipartImagesEdits(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid multipart form", err.Error())
		return
	}
	prompt := strings.TrimSpace(c.PostForm("prompt"))
	if prompt == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "prompt is required", nil)
		return
	}
	images, err := multipartImageValues(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid image", err.Error())
		return
	}
	if len(images) == 0 {
		common.ErrorResponse(c, http.StatusBadRequest, "image is required", nil)
		return
	}
	result, err := runOpenAIImages(c, prompt, images, imageOptions{
		Model:          c.PostForm("model"),
		N:              intFromString(c.PostForm("n")),
		Size:           c.PostForm("size"),
		Quality:        c.PostForm("quality"),
		ResponseFormat: c.PostForm("response_format"),
	})
	if err != nil {
		common.ErrorResponse(c, http.StatusBadGateway, "image edit failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

type imageOptions struct {
	Model          string
	N              int
	Size           string
	Quality        string
	ResponseFormat string
}

func runOpenAIImages(c *gin.Context, prompt string, images []string, opts imageOptions) (*ImagesResponse, error) {
	n := opts.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		return nil, fmt.Errorf("n must be less than or equal to 4")
	}
	tool := normalizeCodexImageTool(responses.Tool{
		Type:         "image_generation",
		Model:        opts.Model,
		Size:         opts.Size,
		Quality:      opts.Quality,
		OutputFormat: outputFormatFromResponseFormat(opts.ResponseFormat),
	}, len(images) > 0)
	items := make([]ImagesRespItem, 0, n)
	for i := 0; i < n; i++ {
		completed, err := collectOpenAIImageResponse(c, prompt, images, tool)
		if err != nil {
			return nil, err
		}
		b64, revised := imageResultFromCompleted(completed)
		if b64 == "" {
			return nil, fmt.Errorf("upstream completed without generating images")
		}
		items = append(items, openAIImageItem(b64, revised, opts.ResponseFormat))
	}
	return &ImagesResponse{Created: time.Now().Unix(), Data: items}, nil
}

func collectOpenAIImageResponse(c *gin.Context, prompt string, images []string, tool responses.Tool) (map[string]interface{}, error) {
	return collectConversationImageResponse(c, prompt, images, tool)
}

func collectConversationImageResponse(c *gin.Context, prompt string, images []string, tool responses.Tool) (map[string]interface{}, error) {
	if len(images) == 0 {
		return collectAuroraConversationImageResponse(c, prompt, tool)
	}
	chatReq := imageConversationRequest(prompt, images, tool)
	resp, accessToken, err := sendChatRequestForModel(c, chatReq, imageRoutingModel(tool.Model))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return nil, fmt.Errorf("upstream returned status %d: %s (token=%s)", resp.StatusCode, string(body), maskUpstreamToken(accessToken))
	}
	return collectConversationImageFromSSE(resp.Body, nil)
}

func collectAuroraConversationImageResponse(c *gin.Context, prompt string, tool responses.Tool) (map[string]interface{}, error) {
	result, err := executeWithModelCandidates(c, imageRoutingModel(tool.Model), func(backend *chatgpt_backend.Client) (*http.Response, error) {
		chatReq := imageConversationRequest(prompt, nil, tool)
		applyChatTargetDefaults(backend, chatReq)
		applyFConversationPayloadDefaults(chatReq)
		chatReq.ClientPrepareState = "none"
		conduitToken, err := prepareFConversation(backend, backend.BaseURL+"/backend-api/f/conversation", chatReq)
		if err != nil {
			return nil, err
		}
		payload := map[string]interface{}{
			"action":                   "next",
			"messages":                 auroraImageMessages(prompt),
			"parent_message_id":        uuid.New().String(),
			"model":                    imageChatModel(tool.Model),
			"client_prepare_state":     "sent",
			"timezone_offset_min":      chatReq.TimeZoneOffsetMin,
			"timezone":                 chatReq.Timezone,
			"conversation_mode":        map[string]string{"kind": "primary_assistant"},
			"enable_message_followups": true,
			"system_hints":             []string{},
			"supports_buffering":       true,
			"supported_encodings":      []string{"v1"},
			"client_contextual_info": map[string]interface{}{
				"is_dark_mode":      false,
				"time_since_loaded": 1200,
				"page_height":       1072,
				"page_width":        1724,
				"pixel_ratio":       1.2,
				"screen_height":     1440,
				"screen_width":      2560,
				"app_name":          "chatgpt.com",
			},
			"paragen_cot_summary_display_override": "allow",
			"force_parallel_switch":                "auto",
			"thinking_effort":                      "standard",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		upstreamURL := backend.BaseURL + "/backend-api/f/conversation"
		headers, cookies := backend.Headers(upstreamURL)
		headers.Set("accept", "text/event-stream")
		headers.Set("content-type", "application/json")
		applySentinelHeaders(headers, backend, true)
		if conduitToken != "" {
			headers.Set("x-conduit-token", conduitToken)
		}
		resp, err := backend.HTTP.Request(tls_client_httpi.POST, upstreamURL, headers, cookies, strings.NewReader(string(body)))
		if err != nil {
			return nil, fmt.Errorf("upstream request failed: %w", err)
		}
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	defer result.Response.Body.Close()
	if result.Response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(result.Response.Body, 64*1024))
		return nil, fmt.Errorf("upstream returned status %d: %s (token=%s)", result.Response.StatusCode, string(body), maskUpstreamToken(result.Token))
	}
	return collectConversationImageFromSSE(result.Response.Body, result.Backend)
}

func auroraImageMessages(prompt string) []map[string]interface{} {
	return []map[string]interface{}{{
		"id":          uuid.New().String(),
		"author":      map[string]string{"role": "user"},
		"create_time": time.Now().Unix(),
		"content":     map[string]interface{}{"content_type": "text", "parts": []string{prompt}},
		"metadata": map[string]interface{}{
			"developer_mode_connector_ids": []interface{}{},
			"selected_github_repos":        []interface{}{},
			"selected_all_github_repos":    false,
				"system_hints":                 []string{},
			"serialization_metadata":       map[string]interface{}{"custom_symbol_offsets": []interface{}{}},
		},
	}}
}

func imageConversationRequest(prompt string, images []string, tool responses.Tool) *chat.Request {
	content := interface{}(prompt)
	if len(images) > 0 {
		parts := make([]interface{}, 0, len(images)+1)
		for _, image := range images {
			parts = append(parts, map[string]interface{}{"type": "input_image", "image_url": normalizeImageDataURL(image)})
		}
		parts = append(parts, prompt)
		content = parts
	}
	req := completions.BuildChatRequest(&completions.ApiReq{
		Model: imageChatModel(tool.Model),
		Messages: []completions.ApiMessage{{
			Role:    "user",
			Content: content,
		}},
		ParentMessageId: uuid.New().String(),
	})
	req.SystemHints = []string{}
	if len(req.Messages) > 0 {
		req.Messages[0].Metadata = map[string]interface{}{
			"developer_mode_connector_ids": []interface{}{},
			"selected_github_repos":        []interface{}{},
			"selected_all_github_repos":    false,
			"system_hints":                 []string{},
			"serialization_metadata":       map[string]interface{}{"custom_symbol_offsets": []interface{}{}},
		}
	}
	return req
}

func imageChatModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || strings.HasPrefix(model, "dall-e") {
		model = "gpt-image-2"
	}
	if model == "gpt-image-2" || strings.HasPrefix(model, "gpt-image") {
		return "auto"
	}
	return model
}

func imageRoutingModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || strings.HasPrefix(model, "dall-e") {
		return "gpt-image-2"
	}
	return model
}

func collectConversationImageFromSSE(body io.Reader, backend *chatgpt_backend.Client) (map[string]interface{}, error) {
	reader := bufio.NewReader(body)
	var lastMessage string
	var conversationID string
	var pendingFileID string
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
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if id := strings.TrimSpace(responseStringValue(event["conversation_id"], "")); id != "" && conversationID == "" {
			conversationID = id
		}
		if fileID := strings.TrimSpace(findImageFileID(event)); fileID != "" {
			pendingFileID = fileID
		}
		if upstreamErr := strings.TrimSpace(responseStringValue(event["error"], "")); upstreamErr != "" {
			return nil, fmt.Errorf("upstream image generation failed: %s", upstreamErr)
		}
		if message := responseMapValue(event["message"]); len(message) > 0 {
			if status := strings.TrimSpace(responseStringValue(message["status"], "")); status == "failed" {
				return nil, fmt.Errorf("upstream image generation failed")
			}
		}
		if b64, revised := imageResultFromCompleted(event); b64 != "" {
			return conversationImageCompleted(b64, revised), nil
		}
		if backend != nil && conversationID != "" && pendingFileID != "" {
			if b64, revised, err := downloadConversationImageFile(backend, conversationID, pendingFileID); err == nil && b64 != "" {
				return conversationImageCompleted(b64, revised), nil
			}
		}
		if text := assistantRawText(event, lastMessage); text != "" {
			lastMessage = text
		}
	}
	if backend != nil && conversationID != "" {
		if pendingFileID != "" {
			if b64, revised, err := downloadConversationImageFile(backend, conversationID, pendingFileID); err == nil && b64 != "" {
				return conversationImageCompleted(b64, revised), nil
			}
		}
		if completed, err := pollConversationImageResult(backend, conversationID); err == nil {
			return completed, nil
		}
	}
	if lastMessage != "" {
		return nil, fmt.Errorf("upstream completed without image: %s", lastMessage)
	}
	return nil, fmt.Errorf("upstream completed without generating images")
}

func pollConversationImageResult(backend *chatgpt_backend.Client, conversationID string) (map[string]interface{}, error) {
	var lastErr error
	for i := 0; i < 45; i++ {
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		if err := waitConversationAsyncStatus(backend, conversationID); err != nil {
			lastErr = err
		}
		conversation, err := fetchConversation(backend, conversationID)
		if err != nil {
			lastErr = err
			continue
		}
		if b64, revised := imageResultFromCompleted(conversation); b64 != "" {
			return conversationImageCompleted(b64, revised), nil
		}
		if fileID := strings.TrimSpace(findImageFileID(conversation)); fileID != "" {
			if b64, revised, err := downloadConversationImageFile(backend, conversationID, fileID); err == nil && b64 != "" {
				return conversationImageCompleted(b64, revised), nil
			} else if err != nil {
				lastErr = err
			}
		}
		if message := findImageGenerationError(conversation); message != "" {
			return nil, fmt.Errorf("upstream image generation failed: %s", message)
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("upstream completed without generating images")
}

func waitConversationAsyncStatus(backend *chatgpt_backend.Client, conversationID string) error {
	path := "/backend-api/conversation/" + conversationID + "/async-status"
	url := backend.BaseURL + path
	headers, cookies := backend.Headers(url)
	headers.Set("accept", "application/json")
	headers.Set("content-type", "application/json")
	resp, err := backend.HTTP.Request(tls_client_httpi.POST, url, headers, cookies, strings.NewReader(`{"status":null}`))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("async status failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func fetchConversation(backend *chatgpt_backend.Client, conversationID string) (map[string]interface{}, error) {
	path := "/backend-api/conversation/" + conversationID
	url := backend.BaseURL + path
	headers, cookies := backend.Headers(url)
	headers.Set("accept", "application/json")
	headers.Set("content-type", "application/json")
	resp, err := backend.HTTP.Request(tls_client_httpi.GET, url, headers, cookies, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get conversation failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findImageGenerationError(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}:
		itemType := strings.TrimSpace(responseStringValue(v["type"], ""))
		if itemType == "content_policy_violation" || itemType == "content_policy_error" {
			if message := strings.TrimSpace(responseStringValue(v["message"], "")); message != "" {
				return message
			}
			return "Image generation was rejected by the upstream content policy."
		}
		if code := strings.ToLower(strings.TrimSpace(responseStringValue(v["code"], ""))); strings.Contains(code, "content_policy") {
			if message := strings.TrimSpace(responseStringValue(v["message"], "")); message != "" {
				return message
			}
			return "Image generation was rejected by the upstream content policy."
		}
		for _, child := range v {
			if message := findImageGenerationError(child); message != "" {
				return message
			}
		}
	case []interface{}:
		for _, child := range v {
			if message := findImageGenerationError(child); message != "" {
				return message
			}
		}
	}
	return ""
}

func conversationImageCompleted(b64 string, revised string) map[string]interface{} {
	item := map[string]interface{}{
		"type":           "image_generation_call",
		"status":         "completed",
		"result":         b64,
		"revised_prompt": revised,
	}
	return map[string]interface{}{
		"id":      "resp_" + uuid.New().String(),
		"object":  "response",
		"created": time.Now().Unix(),
		"status":  "completed",
		"output":  []interface{}{item},
	}
}

func maskUpstreamToken(token string) string {
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if len(token) <= 12 {
		return "***"
	}
	return token[:8] + "..." + token[len(token)-4:]
}

func openAIImageItem(b64 string, revisedPrompt string, responseFormat string) ImagesRespItem {
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		return ImagesRespItem{URL: "data:image/png;base64," + b64, RevisedPrompt: revisedPrompt}
	}
	return ImagesRespItem{B64JSON: b64, RevisedPrompt: revisedPrompt}
}

func outputFormatFromResponseFormat(responseFormat string) string {
	responseFormat = strings.TrimSpace(strings.ToLower(responseFormat))
	if responseFormat == "" || responseFormat == "b64_json" || responseFormat == "url" {
		return "png"
	}
	return responseFormat
}

func imageResultFromCompleted(value interface{}) (string, string) {
	switch v := value.(type) {
	case map[string]interface{}:
		result := strings.TrimSpace(responseStringValue(v["result"], ""))
		if isImageResultValue(result) {
			return stripImageDataURL(result), responseStringValue(v["revised_prompt"], "")
		}
		for _, key := range []string{"b64_json", "image", "url"} {
			if text := strings.TrimSpace(responseStringValue(v[key], "")); text != "" {
				if isImageResultValue(text) || key == "b64_json" {
					return stripImageDataURL(text), responseStringValue(v["revised_prompt"], "")
				}
			}
		}
		for _, child := range v {
			if b64, revised := imageResultFromCompleted(child); b64 != "" {
				if revised == "" {
					revised = responseStringValue(v["revised_prompt"], "")
				}
				return b64, revised
			}
		}
	case []interface{}:
		for _, child := range v {
			if b64, revised := imageResultFromCompleted(child); b64 != "" {
				return b64, revised
			}
		}
	}
	return "", ""
}

func isImageResultValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "data:image/") {
		return true
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return strings.Contains(value, "/backend-api/estuary/content") || strings.Contains(value, "image") || strings.Contains(value, ".png") || strings.Contains(value, ".jpg") || strings.Contains(value, ".jpeg") || strings.Contains(value, ".webp")
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return false
	}
	if len(value) < 256 {
		return false
	}
	for _, ch := range value {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '+' || ch == '/' || ch == '=' || ch == '\n' || ch == '\r' {
			continue
		}
		return false
	}
	return true
}

func findImageFileID(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"file_id", "fileId"} {
			if fileID := strings.TrimSpace(responseStringValue(v[key], "")); strings.HasPrefix(fileID, "file_") {
				return fileID
			}
		}
		if pointer := strings.TrimSpace(responseStringValue(v["asset_pointer"], "")); pointer != "" {
			if fileID := fileIDFromPointer(pointer); fileID != "" {
				return fileID
			}
		}
		for _, child := range v {
			if fileID := findImageFileID(child); fileID != "" {
				return fileID
			}
		}
	case []interface{}:
		for _, child := range v {
			if fileID := findImageFileID(child); fileID != "" {
				return fileID
			}
		}
	}
	return ""
}

func fileIDFromPointer(pointer string) string {
	pointer = strings.TrimSpace(pointer)
	for _, prefix := range []string{"file-service://", "sediment://"} {
		if strings.HasPrefix(pointer, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(pointer, prefix))
		}
	}
	return ""
}

func downloadConversationImageFile(backend *chatgpt_backend.Client, conversationID string, fileID string) (string, string, error) {
	meta, err := fetchConversationFileDownload(backend, conversationID, fileID)
	if err != nil {
		return "", "", err
	}
	if meta.DownloadURL == "" {
		return "", "", fmt.Errorf("conversation image download_url is empty")
	}
	data, err := fetchConversationFileBinary(backend, meta.DownloadURL)
	if err != nil {
		return "", "", err
	}
	if len(data) == 0 {
		return "", "", fmt.Errorf("conversation image file is empty")
	}
	return base64.StdEncoding.EncodeToString(data), meta.FileName, nil
}

type conversationFileDownload struct {
	DownloadURL string `json:"download_url"`
	FileName    string `json:"file_name"`
}

func fetchConversationFileDownload(backend *chatgpt_backend.Client, conversationID string, fileID string) (*conversationFileDownload, error) {
	path := fmt.Sprintf("/backend-api/files/download/%s?conversation_id=%s&inline=false&download_intent=false", fileID, conversationID)
	url := backend.BaseURL + path
	headers, cookies := backend.Headers(url)
	headers.Set("accept", "application/json")
	resp, err := backend.HTTP.Request(tls_client_httpi.GET, url, headers, cookies, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("files/download failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	result := &conversationFileDownload{}
	if err := json.Unmarshal(body, result); err != nil {
		return nil, err
	}
	return result, nil
}

func fetchConversationFileBinary(backend *chatgpt_backend.Client, url string) ([]byte, error) {
	headers, cookies := backend.Headers(url)
	headers.Set("accept", "image/*,*/*")
	resp, err := backend.HTTP.Request(tls_client_httpi.GET, url, headers, cookies, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("estuary content download failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return body, nil
}

func stripImageDataURL(value string) string {
	value = strings.TrimSpace(value)
	if comma := strings.Index(value, ","); strings.HasPrefix(value, "data:image/") && comma >= 0 {
		return strings.TrimSpace(value[comma+1:])
	}
	return value
}

func imageValuesFromJSON(values ...interface{}) []string {
	images := make([]string, 0)
	for _, value := range values {
		collectImageValues(value, &images)
	}
	return images
}

func collectImageValues(value interface{}, images *[]string) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			*images = append(*images, strings.TrimSpace(v))
		}
	case []interface{}:
		for _, item := range v {
			collectImageValues(item, images)
		}
	case map[string]interface{}:
		for _, key := range []string{"image_url", "url", "base64", "b64_json"} {
			if text := responseStringValue(v[key], ""); strings.TrimSpace(text) != "" {
				*images = append(*images, strings.TrimSpace(text))
				return
			}
		}
		if source, ok := v["source"].(map[string]interface{}); ok {
			collectImageValues(source["data"], images)
		}
	}
}

func multipartImageValues(c *gin.Context) ([]string, error) {
	form := c.Request.MultipartForm
	if form == nil {
		return nil, nil
	}
	images := make([]string, 0)
	for _, key := range []string{"image", "image[]", "images", "images[]"} {
		for _, header := range form.File[key] {
			file, err := header.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				return nil, err
			}
			if len(data) > 0 {
				images = append(images, base64.StdEncoding.EncodeToString(data))
			}
		}
	}
	for _, key := range []string{"image_url", "image_url[]"} {
		for _, value := range form.Value[key] {
			if strings.TrimSpace(value) != "" {
				images = append(images, strings.TrimSpace(value))
			}
		}
	}
	return images, nil
}

func intFromString(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(value, "%d", &n)
	return n
}
