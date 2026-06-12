package service

import (
	"bytes"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/result"
	"chat2api/app/types/completions"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

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

type modelProbeCache struct {
	mu        sync.RWMutex
	models    []probedModel
	updatedAt int64
}

var modelsCache = &modelProbeCache{}

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

func cachedAvailableModels() []probedModel {
	modelsCache.mu.RLock()
	defer modelsCache.mu.RUnlock()
	if len(modelsCache.models) == 0 {
		out := make([]probedModel, 0, 1)
		out = append(out, probedModel{ID: "auto", Object: "model", OwnedBy: "chatgpt-web", Available: true})
		return out
	}
	out := make([]probedModel, 0, len(modelsCache.models))
	for _, model := range modelsCache.models {
		if model.Available {
			out = append(out, model)
		}
	}
	if len(out) == 0 {
		out = append(out, probedModel{ID: "auto", Object: "model", OwnedBy: "chatgpt-web", Available: true})
	}
	return out
}

func setModelProbeResults(results []probedModel) {
	modelsCache.mu.Lock()
	defer modelsCache.mu.Unlock()
	modelsCache.models = results
	modelsCache.updatedAt = time.Now().Unix()
}

func getModelProbeResults() ([]probedModel, int64) {
	modelsCache.mu.RLock()
	defer modelsCache.mu.RUnlock()
	out := make([]probedModel, len(modelsCache.models))
	copy(out, modelsCache.models)
	return out, modelsCache.updatedAt
}

func probeModelAvailability(c *gin.Context, model string) probedModel {
	result := probedModel{ID: model, Object: "model", OwnedBy: "chatgpt-web"}
	backend, err := chatgpt_backend.New(c.Request.Header.Get("Authorization"), chatgpt_backend.Retry())
	if err != nil {
		result.Reason = err.Error()
		return result
	}
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
	models := cachedAvailableModels()
	data := make([]gin.H, 0, len(models))
	for _, model := range models {
		data = append(data, gin.H{
			"id":       model.ID,
			"object":   model.Object,
			"owned_by": model.OwnedBy,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

func AdminProbeModels(c *gin.Context) {
	candidates := defaultModelCandidates()
	results := make([]probedModel, 0, len(candidates))
	for _, candidate := range candidates {
		results = append(results, probeModelAvailability(c, strings.TrimSpace(candidate)))
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Available == results[j].Available {
			return results[i].ID < results[j].ID
		}
		return results[i].Available && !results[j].Available
	})
	setModelProbeResults(results)
	_, updatedAt := getModelProbeResults()
	result.New(c, "admin_probe_models").AssertSuccessful(gin.H{
		"models":      results,
		"updated_at":  updatedAt,
		"available":   countAvailableModels(results),
		"total":       len(results),
	}, nil)
}

func AdminGetModels(c *gin.Context) {
	models, updatedAt := getModelProbeResults()
	result.New(c, "admin_get_models").AssertSuccessful(gin.H{
		"models":     models,
		"updated_at": updatedAt,
		"available":  countAvailableModels(models),
		"total":      len(models),
	}, nil)
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
