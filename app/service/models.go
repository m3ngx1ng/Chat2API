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
	candidates := defaultModelCandidates()
	results := make([]probedModel, 0, len(candidates))
	for _, candidate := range candidates {
		results = append(results, probeModelAvailabilityWithClient(backend, strings.TrimSpace(candidate)))
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
