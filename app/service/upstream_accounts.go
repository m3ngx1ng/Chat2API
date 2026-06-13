package service

import (
	"bytes"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/conf"
	"chat2api/app/token_pool"
	"chat2api/app/types/chat"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/gin-gonic/gin"
)

type upstreamAttemptResult struct {
	Response *http.Response
	Backend  *chatgpt_backend.Client
	Token    string
}

func requestModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	return model
}

func sendChatRequest(c *gin.Context, chatReq *chat.Request) (*http.Response, string, error) {
	result, err := sendChatRequestWithBackend(c, chatReq, chatReq.Model)
	if err != nil {
		return nil, "", err
	}
	return result.Response, result.Token, nil
}

func sendChatRequestForModel(c *gin.Context, chatReq *chat.Request, routingModel string) (*http.Response, string, error) {
	result, err := sendChatRequestWithBackend(c, chatReq, routingModel)
	if err != nil {
		return nil, "", err
	}
	return result.Response, result.Token, nil
}

func sendChatRequestWithBackend(c *gin.Context, chatReq *chat.Request, routingModel string) (*upstreamAttemptResult, error) {
	model := requestModel(routingModel)
	return executeWithModelCandidates(c, model, func(backend *chatgpt_backend.Client) (*http.Response, error) {
		requestCopy, err := cloneChatRequest(chatReq)
		if err != nil {
			return nil, err
		}
		if err := prepareChatVisionInputs(backend, requestCopy); err != nil {
			return nil, err
		}
		applyChatTargetDefaults(backend, requestCopy)
		upstreamURL := backend.ChatURL
		if shouldUseFConversation(backend) {
			upstreamURL = backend.BaseURL + "/backend-api/f/conversation"
			applyFConversationPayloadDefaults(requestCopy)
		}
		conduitToken, err := prepareFConversation(backend, upstreamURL, requestCopy)
		if err != nil {
			return nil, err
		}
		body, err := common.Struct2BytesBuffer(requestCopy)
		if err != nil {
			return nil, err
		}
		headers, cookies := backend.Headers(upstreamURL)
		headers.Set("accept", "text/event-stream")
		headers.Set("content-type", "application/json")
		applySentinelHeaders(headers, backend, true)
		if conduitToken != "" {
			headers.Set("x-conduit-token", conduitToken)
		}
		response, err := backend.HTTP.Request(tls_client_httpi.POST, upstreamURL, headers, cookies, body)
		if err != nil {
			return nil, fmt.Errorf("upstream request failed: %w", err)
		}
		return response, nil
	})
}

func executeWithModelCandidates(c *gin.Context, model string, execute func(backend *chatgpt_backend.Client) (*http.Response, error)) (*upstreamAttemptResult, error) {
	model = requestModel(model)
	authHeader := c.Request.Header.Get("Authorization")
	appConf := conf.GetApp()
	localToken := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(authHeader), "Bearer "))
	directAccessToken, directAccessTokenOK := appConf.DirectAccessToken(localToken)
	if shouldUseDirectUpstreamToken(authHeader, directAccessToken, directAccessTokenOK) {
		backend, err := chatgpt_backend.NewForModel(authHeader, model, chatgpt_backend.Retry())
		if err != nil {
			return nil, err
		}
		response, err := execute(backend)
		if err != nil {
			return nil, err
		}
		return &upstreamAttemptResult{Response: response, Backend: backend, Token: backend.AccAuth}, nil
	}

	candidates := chatgpt_backend.AccessTokenCandidates(model)
	if len(candidates) == 0 {
		backend, err := chatgpt_backend.NewForModel(authHeader, model, chatgpt_backend.Retry())
		if err != nil {
			return nil, err
		}
		response, err := execute(backend)
		if err != nil {
			return nil, err
		}
		return &upstreamAttemptResult{Response: response, Backend: backend, Token: backend.AccAuth}, nil
	}

	var lastErr error
	for index, candidate := range candidates {
		backend, err := chatgpt_backend.NewWithAccessToken(candidate, chatgpt_backend.Retry())
		if err != nil {
			lastErr = err
			markCandidateUnavailable(candidate, nil)
			continue
		}
		response, err := execute(backend)
		if err == nil {
			if shouldRetryResponseForNextAccount(response) && index < len(candidates)-1 {
				markCandidateUnavailable(candidate, response)
				lastErr = responseStatusError(response)
				closeResponseBody(response)
				continue
			}
			return &upstreamAttemptResult{Response: response, Backend: backend, Token: backend.AccAuth}, nil
		}
		lastErr = err
		if !shouldRetryUpstreamError(err) || index == len(candidates)-1 {
			return nil, err
		}
		markCandidateUnavailable(candidate, nil)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no upstream account available for model %q", model)
}

func shouldUseDirectUpstreamToken(authHeader string, directAccessToken string, directAccessTokenOK bool) bool {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return false
	}
	if directAccessTokenOK && strings.TrimSpace(directAccessToken) != "" {
		return true
	}
	if strings.HasPrefix(authHeader, "Bearer eyJhbGciOiJSUzI1NiI") {
		return true
	}
	return false
}

func cloneChatRequest(chatReq *chat.Request) (*chat.Request, error) {
	data, err := common.Struct2Bytes(chatReq)
	if err != nil {
		return nil, err
	}
	cloned := &chat.Request{}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func shouldRetryUpstreamError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	for _, token := range []string{
		"upstream request failed",
		"chat requirements failed: status=401",
		"chat requirements failed: status=403",
		"chat requirements failed: status=408",
		"chat requirements failed: status=409",
		"chat requirements failed: status=429",
		"chat requirements failed: status=500",
		"chat requirements failed: status=502",
		"chat requirements failed: status=503",
		"chat requirements failed: status=504",
		"force login required",
		"proof token failed",
		"missing chat requirements token",
		"model",
		"unavailable",
		"not found",
	} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func shouldRetryResponseForNextAccount(response *http.Response) bool {
	if response == nil {
		return false
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusConflict {
		return true
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return true
	}
	if response.StatusCode != http.StatusBadRequest {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return false
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	message := strings.ToLower(string(body))
	for _, token := range []string{"model", "unavailable", "not found", "does not exist", "unsupported"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func responseStatusError(response *http.Response) error {
	if response == nil {
		return fmt.Errorf("upstream response is nil")
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	response.Body = io.NopCloser(bytes.NewReader(body))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return fmt.Errorf("upstream returned status %d: %s", response.StatusCode, message)
}

func markCandidateUnavailable(candidate *token_pool.AccessToken, response *http.Response) {
	if candidate == nil || candidate.Token == "" {
		return
	}
	if response != nil && response.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		response.Body = io.NopCloser(bytes.NewReader(body))
		token_pool.GetAccessTokenPool().SetCanUseAt(candidate.Token, rateLimitCanUseAt(response, body))
		return
	}
	token_pool.GetAccessTokenPool().SetCanUseAt(candidate.Token, common.GetTimestampSecond(30))
}

func closeResponseBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}
