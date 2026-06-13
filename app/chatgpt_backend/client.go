package chatgpt_backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"chat2api/app/common"
	"chat2api/app/conf"
	"chat2api/app/constant"
	"chat2api/app/token_pool"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/aurorax-neo/tls_client_httpi/tls_client"
	"github.com/google/uuid"
)

type Client struct {
	HTTP      tls_client_httpi.TCHI
	Auth      *chatRequirements
	AccAuth   string
	BaseURL   string
	ChatURL   string
	UserAgent string
	SessionID string
	Cookies   tls_client_httpi.Cookies
	Pow       Resources
}

type AccessTokenCandidate = token_pool.AccessToken

type chatRequirements struct {
	OaiDeviceID    string    `json:"-"`
	Arkose         challenge `json:"arkose"`
	Turnstile      challenge `json:"turnstile"`
	TurnstileToken string    `json:"-"`
	ProofWork      ProofWork `json:"proofofwork"`
	Token          string    `json:"token"`
	SoToken        string    `json:"so_token"`
	ForceLogin     bool      `json:"force_login"`
}

type challenge struct {
	Required bool   `json:"required"`
	Dx       string `json:"dx"`
}

func New(token string, retry int) (*Client, error) {
	return NewForModel(token, "", retry)
}

func NewForModel(token string, model string, retry int) (*Client, error) {
	token = strings.TrimSpace(token)
	localToken := strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	appConf := conf.GetApp()
	if accessToken, ok := appConf.DirectAccessToken(localToken); ok {
		return newClient("Bearer "+accessToken, "")
	}
	if strings.HasPrefix(token, "Bearer eyJhbGciOiJSUzI1NiI") {
		return newClient(token, "")
	}
	if !token_pool.GetAccessTokenPool().IsEmpty() {
		accessToken := token_pool.GetAccessTokenPool().GetAccessTokenByModel(model)
		if accessToken == nil || accessToken.Token == "" {
			return nil, fmt.Errorf("access token pool has no available account for model %q", strings.TrimSpace(model))
		}
		return NewWithAccessToken(accessToken, retry)
	}
	if strings.HasPrefix(localToken, "sk-") {
		return nil, fmt.Errorf("access token pool is empty")
	}
	client, err := newClient(token, "")
	if client == nil && retry > 0 {
		return NewForModel(token, model, retry-1)
	}
	return client, err
}

func NewWithAccessToken(accessToken *AccessTokenCandidate, retry int) (*Client, error) {
	if accessToken == nil || accessToken.Token == "" {
		return nil, fmt.Errorf("access token is empty")
	}
	client, err := newClient(accessToken.Token, accessToken.Proxy)
	if client == nil && retry > 0 {
		return NewWithAccessToken(accessToken, retry-1)
	}
	return client, err
}

func AccessTokenCandidates(model string) []*AccessTokenCandidate {
	return token_pool.GetAccessTokenPool().GetAccessTokensByModel(model)
}

func NewExplicit(token string, accountProxy string) (*Client, error) {
	return newClient(strings.TrimSpace(token), strings.TrimSpace(accountProxy))
}

func newClient(token string, accountProxy string) (*Client, error) {
	appConf := conf.GetApp()
	baseURL := strings.TrimRight(appConf.ChatGPTBaseUrl, "/")
	if baseURL == "" {
		baseURL = "https://chatgpt.com"
	}
	c := &Client{
		HTTP:      tls_client.NewClient(tls_client.NewClientOptions(300, common.GetClientProfile())),
		Auth:      &chatRequirements{OaiDeviceID: uuid.New().String()},
		BaseURL:   baseURL,
		ChatURL:   baseURL + "/backend-anon/conversation",
		UserAgent: common.GetUa(),
		SessionID: uuid.New().String(),
	}
	if c.HTTP == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	if strings.HasPrefix(token, "Bearer ") {
		c.AccAuth = token
		c.ChatURL = baseURL + "/backend-api/conversation"
	}
	proxy := strings.TrimSpace(accountProxy)
	if proxy == "" {
		proxy = strings.TrimSpace(appConf.Proxy)
	}
	if proxy != "" {
		if err := c.HTTP.SetProxy(proxy); err != nil {
			return nil, err
		}
	}
	c.loadPowResources()
	if err := c.loadRequirements(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) Headers(url string) (tls_client_httpi.Headers, tls_client_httpi.Cookies) {
	headers := tls_client_httpi.Headers{}
	path := strings.TrimPrefix(url, c.BaseURL)
	headers.Set("accept", "*/*")
	headers.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7")
	headers.Set("origin", c.BaseURL)
	headers.Set("referer", c.BaseURL+"/")
	headers.Set("cache-control", "no-cache")
	headers.Set("pragma", "no-cache")
	headers.Set("priority", "u=1, i")
	headers.Set("sec-ch-ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	headers.Set("sec-ch-ua-arch", `"x86"`)
	headers.Set("sec-ch-ua-bitness", `"64"`)
	headers.Set("sec-ch-ua-full-version", `"143.0.3650.96"`)
	headers.Set("sec-ch-ua-full-version-list", `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`)
	headers.Set("sec-ch-ua-mobile", "?0")
	headers.Set("sec-ch-ua-model", `""`)
	headers.Set("sec-ch-ua-platform", `"Windows"`)
	headers.Set("sec-ch-ua-platform-version", `"19.0.0"`)
	headers.Set("sec-fetch-dest", "empty")
	headers.Set("sec-fetch-mode", "cors")
	headers.Set("sec-fetch-site", "same-origin")
	headers.Set("user-agent", c.UserAgent)
	headers.Set("oai-device-id", c.Auth.OaiDeviceID)
	headers.Set("oai-session-id", c.SessionID)
	headers.Set("oai-language", "zh-CN")
	headers.Set("oai-client-version", "prod-3b8f2c1740596d77c64c1d3d50205828839b2730")
	headers.Set("oai-client-build-number", "3310101057")
	headers.Set("x-openai-target-path", path)
	headers.Set("x-openai-target-route", path)
	if c.AccAuth != "" {
		headers.Set("authorization", c.AccAuth)
	}
	return headers, c.Cookies
}

func (c *Client) IsAuthenticated() bool {
	return c.AccAuth != ""
}

func (c *Client) ChatTimezone() (string, int) {
	if c.IsAuthenticated() {
		return "Asia/Shanghai", -480
	}
	return "America/Los_Angeles", 480
}

func (c *Client) loadPowResources() {
	headers, cookies := c.Headers(c.BaseURL + "/")
	headers.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	response, err := c.HTTP.Request(tls_client_httpi.GET, c.BaseURL+"/", headers, cookies, nil)
	if err != nil {
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		return
	}
	c.Pow = ParseResources(string(body))
}

func (c *Client) loadRequirements() error {
	authURL := c.BaseURL + "/backend-anon/sentinel/chat-requirements"
	if c.AccAuth != "" {
		authURL = c.BaseURL + "/backend-api/sentinel/chat-requirements"
	}
	requirementsToken := LegacyRequirementsToken(c.UserAgent, c.Pow)
	body := bytes.NewBufferString(`{"p":"` + requirementsToken + `"}`)
	headers, cookies := c.Headers(authURL)
	headers.Set("content-type", "application/json")
	response, err := c.HTTP.Request(tls_client_httpi.POST, authURL, headers, cookies, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("chat requirements failed: status=%d body=%s", response.StatusCode, string(detail))
	}
	if err := json.NewDecoder(response.Body).Decode(&c.Auth); err != nil {
		return err
	}
	if c.Auth.ForceLogin {
		common.SubUpdateThreshold()
		return fmt.Errorf("force login required")
	}
	if c.Auth.Arkose.Required {
		return fmt.Errorf("arkose token is required")
	}
	if c.Auth.Turnstile.Required && c.Auth.Turnstile.Dx != "" {
		sourceP := ""
		if c.AccAuth == "" {
			sourceP = requirementsToken
		}
		c.Auth.TurnstileToken = Solve(c.Auth.Turnstile.Dx, sourceP)
		if c.Auth.TurnstileToken == "" {
			fallbackP := requirementsToken
			if sourceP == requirementsToken {
				fallbackP = ""
			}
			c.Auth.TurnstileToken = Solve(c.Auth.Turnstile.Dx, fallbackP)
		}
	}
	if c.Auth.ProofWork.Required {
		c.Auth.ProofWork.Ospt = CalcProofToken(c.Auth.ProofWork.Seed, c.Auth.ProofWork.Difficulty, c.UserAgent, c.Pow)
		if c.Auth.ProofWork.Ospt == "" {
			return fmt.Errorf("proof token failed")
		}
	}
	if c.Auth.Token == "" {
		return fmt.Errorf("missing chat requirements token")
	}
	return nil
}

func Retry() int {
	return constant.ReTry
}
