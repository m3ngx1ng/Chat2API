package service

import (
	"chat2api/app/types/completions"

	"github.com/gin-gonic/gin"
)

func runResponsesTextChat(c *gin.Context, apiReq *completions.ApiReq, streamResponses bool) (*chatResult, error) {
	chatReq := completions.BuildChatRequest(apiReq)
	upstreamResult, err := sendChatRequestWithBackend(c, chatReq, chatReq.Model)
	if err != nil {
		return nil, err
	}
	resp := upstreamResult.Response
	accessToken := upstreamResult.Token
	defer resp.Body.Close()
	if handleResponseError(c, resp, accessToken) {
		return nil, nil
	}
	if streamResponses {
		if completions.HasTools(apiReq) {
			return streamResponsesFunctionCallingEvents(c, apiReq, resp)
		}
		_, err := streamResponsesTextEvents(c, apiReq.Model, resp)
		return nil, err
	}
	return handlerResponse(c, apiReq, resp, upstreamResult.Backend)
}
