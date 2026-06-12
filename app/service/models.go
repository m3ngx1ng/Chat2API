package service

import (
	"bytes"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/conf"
	"chat2api/app/result"
	"chat2api/app/types/completions"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type probedModel struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	OwnedBy   string `json:"owned_by"`
	Available bool   `json:"available,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type adminProbeModelsRequest struct {
	AccountID   string `json:"account_id"`
	AccessToken string `json:"access_token"`
	Proxy       string `json:"proxy"`
}

func defaultModelCandidates() []string {
	return []string{
		"auto",
		"gpt-4o",
		"gpt-4.1",
		"gpt-4.1-mini",
		"gpt-4.5",
		"gpt-5",
		"gpt-5.5",
		"o3",
		"o4-mini",
		"gpt-image-2",
	}
}

func normalizeModelNames(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func selectedModelsPayload(models []string) []gin.H {
	data := make([]gin.H, 0, len(models))
	for _, model := range normalizeModelNames(models) {
		data = append(data, gin.H{
			"id":       model,
			"object":   "model",
			"owned_by": "chatgpt-web",
		})
	}
	return data
}

func buildAccountProbeClient(token string, proxy string) (*chatgpt_backend.Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("account access_token is empty")
	}
	return chatgpt_backend.NewExplicit("Bearer "+token, strings.TrimSpace(proxy))
}

func probeModelAvailabilityWithClient(backend *chatgpt_backend.Client, model string) probedModel {
	result := probedModel{ID: model, Object: "model", OwnedBy: "chatgpt-web"}
	req := completions.BuildChatRequest(&completions.ApiReq{
		Model: model,
		Messages: []completions.ApiMessage{{
			Role:    "user",
			Content: "ping",
		}},
		ParentMessageId: uuid.New().String(),
	})
	applyChatTargetDefaults(backend, req)
	upstreamURL := backend.ChatURL
	if shouldUseFConversation(backend) {
		upstreamURL = backend.BaseURL + "/backend-api/f/conversation"
		applyFConversationPayloadDefaults(req)
	}
	conduitToken, err := prepareFConversation(backend, upstreamURL, req)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	body, err := common.Struct2Bytes(req)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	headers, cookies := backend.Headers(upstreamURL)
	headers.Set("accept", "text/event-stream")
	headers.Set("content-type", "application/json")
	applySentinelHeaders(headers, backend, true)
	if conduitToken != "" {
		headers.Set("x-conduit-token", conduitToken)
	}
	resp, err := backend.HTTP.Request(tls_client_httpi.POST, upstreamURL, headers, cookies, bytes.NewReader(body))
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		result.Available = true
		return result
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
		if detail, ok := payload["detail"]; ok && detail != nil {
			result.Reason = fmt.Sprint(detail)
		} else {
			result.Reason = fmt.Sprintf("status=%d", resp.StatusCode)
		}
	} else {
		result.Reason = fmt.Sprintf("status=%d", resp.StatusCode)
	}
	return result
}

func fetchWebAvailableModels(backend *chatgpt_backend.Client) ([]string, error) {
	paths := []string{
		"/backend-api/models",
		"/backend-api/models?history_and_training_disabled=false",
		"/backend-api/models?history_and_training_disabled=true",
	}
	var lastErr error
	for _, path := range paths {
		models, err := fetchWebAvailableModelsFromPath(backend, path)
		if err == nil && len(models) > 0 {
			return models, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no models found from web response")
}

func fetchWebAvailableModelsFromPath(backend *chatgpt_backend.Client, path string) ([]string, error) {
	url := backend.BaseURL + path
	headers, cookies := backend.Headers(url)
	headers.Set("accept", "application/json")
	resp, err := backend.HTTP.Request(tls_client_httpi.GET, url, headers, cookies, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch models failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var payload interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return normalizeModelNames(collectModelSlugs(payload)), nil
}

func collectModelSlugs(node interface{}) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	var walk func(value interface{})
	walk = func(value interface{}) {
		switch typed := value.(type) {
		case map[string]interface{}:
			for key, item := range typed {
				lowerKey := strings.ToLower(strings.TrimSpace(key))
				if text, ok := item.(string); ok && isModelKey(lowerKey) && looksLikeModelSlug(text) {
					text = strings.TrimSpace(text)
					if _, exists := seen[text]; !exists {
						seen[text] = struct{}{}
						result = append(result, text)
					}
				}
				walk(item)
			}
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(node)
	return result
}

func isModelKey(key string) bool {
	return key == "slug" || key == "model_slug" || key == "default_model_slug" || key == "next_model_slug"
}

func looksLikeModelSlug(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.ContainsAny(value, " @/") {
		return false
	}
	return true
}

func modelsFromWebList(models []string) []probedModel {
	items := make([]probedModel, 0, len(models))
	for _, model := range normalizeModelNames(models) {
		items = append(items, probedModel{
			ID:        model,
			Object:    "model",
			OwnedBy:   "chatgpt-web",
			Available: true,
		})
	}
	return items
}

func ModelsList(c *gin.Context) {
	data := selectedModelsPayload(conf.GetApp().SummaryModels())
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

func AdminProbeModels(c *gin.Context) {
	req := adminProbeModelsRequest{}
	jb := result.New(c, "admin_probe_models")
	if jb.BindJson(&req) {
		return
	}
	if strings.TrimSpace(req.AccountID) == "" {
		jb.Error(fmt.Errorf("account_id is required"))
		return
	}
	backend, err := buildAccountProbeClient(req.AccessToken, req.Proxy)
	if jb.AssertError(err) {
		return
	}
	models, err := fetchWebAvailableModels(backend)
	results := make([]probedModel, 0)
	if err == nil && len(models) > 0 {
		results = modelsFromWebList(models)
	} else {
		candidates := defaultModelCandidates()
		results = make([]probedModel, 0, len(candidates))
		for _, candidate := range candidates {
			results = append(results, probeModelAvailabilityWithClient(backend, strings.TrimSpace(candidate)))
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Available == results[j].Available {
			return results[i].ID < results[j].ID
		}
		return results[i].Available && !results[j].Available
	})
	jb.Data = gin.H{
		"account_id":       strings.TrimSpace(req.AccountID),
		"models":           results,
		"available":        countAvailableModels(results),
		"total":            len(results),
		"available_models": extractAvailableModelIDs(results),
		"source":           map[bool]string{true: "chatgpt_web_models", false: "fallback_probe"}[err == nil && len(models) > 0],
	}
	jb.Successful()
}

func AdminGetModels(c *gin.Context) {
	snapshot, err := conf.AdminSnapshot()
	jb := result.New(c, "admin_get_models")
	if jb.AssertError(err) {
		return
	}
	jb.Data = gin.H{
		"summary_models": snapshot.SummaryModels,
	}
	jb.Successful()
}

func countAvailableModels(models []probedModel) int {
	count := 0
	for _, model := range models {
		if model.Available {
			count++
		}
	}
	return count
}

func extractAvailableModelIDs(models []probedModel) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model.Available {
			out = append(out, model.ID)
		}
	}
	return normalizeModelNames(out)
}
