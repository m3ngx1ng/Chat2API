package token_pool

import (
	"chat2api/app/common"
	"strings"
	"sync"
)

var (
	instance *AccessTokenPool
	once     sync.Once
)

type AccessTokenPool struct {
	mu           sync.Mutex
	AccessTokens []*AccessToken
	index        int
}

type AccessToken struct {
	Token     string   `yaml:"token,omitempty"`
	ExpiresAt int64    `yaml:"expires_at,omitempty"`
	Proxy     string   `yaml:"proxy,omitempty"`
	CanUseAt  int64    `yaml:"-"`
	Models    []string `yaml:"-"`
}

func newAccessTokenPool() *AccessTokenPool {
	return &AccessTokenPool{
		AccessTokens: make([]*AccessToken, 0),
		index:        -1,
	}
}

func GetAccessTokenPool() *AccessTokenPool {
	once.Do(func() {
		instance = newAccessTokenPool()
	})
	return instance
}

func (a *AccessTokenPool) AddAccessToken(accessToken *AccessToken) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AccessTokens = append(a.AccessTokens, accessToken)
}

func (a *AccessTokenPool) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AccessTokens = make([]*AccessToken, 0)
	a.index = -1
}

func (a *AccessTokenPool) AppendAccessTokens(accessTokens []*AccessToken) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.AccessTokens = append(a.AccessTokens, accessTokens...)
}

func (a *AccessTokenPool) Size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.AccessTokens)
}

func (a *AccessTokenPool) IsEmpty() bool {
	return a.Size() == 0
}

func (a *AccessTokenPool) CanUseSize() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := common.GetTimestampSecond(0)
	count := 0
	for _, v := range a.AccessTokens {
		if v.CanUseAt <= now && v.ExpiresAt > now {
			count++
		}
	}
	return count
}

func (a *AccessTokenPool) GetToken() string {
	accessToken := a.GetAccessToken()
	if accessToken == nil {
		return ""
	}
	return accessToken.Token
}

func (a *AccessTokenPool) GetAccessToken() *AccessToken {
	return a.GetAccessTokenByModel("")
}

func (a *AccessTokenPool) GetAccessTokenByModel(model string) *AccessToken {
	tokens := a.GetAccessTokensByModel(model)
	if len(tokens) == 0 {
		return nil
	}
	return tokens[0]
}

func (a *AccessTokenPool) GetAccessTokensByModel(model string) []*AccessToken {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.AccessTokens) == 0 {
		return nil
	}

	now := common.GetTimestampSecond(0)
	total := len(a.AccessTokens)
	tokens := make([]*AccessToken, 0, total)
	selectedIndex := -1

	for i := 0; i < total; i++ {
		index := (a.index + 1 + i) % total
		token := a.AccessTokens[index]
		if token.CanUseAt <= now && token.ExpiresAt > now && accessTokenSupportsModel(token, model) {
			if selectedIndex < 0 {
				selectedIndex = index
			}
			tokens = append(tokens, token)
		}
	}
	if selectedIndex >= 0 {
		a.index = selectedIndex
	}

	return tokens
}

func (a *AccessTokenPool) CanUseSizeForModel(model string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := common.GetTimestampSecond(0)
	count := 0
	for _, token := range a.AccessTokens {
		if token.CanUseAt <= now && token.ExpiresAt > now && accessTokenSupportsModel(token, model) {
			count++
		}
	}
	return count
}

func accessTokenSupportsModel(token *AccessToken, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" || strings.EqualFold(model, "auto") {
		return true
	}
	if len(token.Models) == 0 {
		return true
	}
	for _, candidate := range token.Models {
		if strings.EqualFold(strings.TrimSpace(candidate), model) {
			return true
		}
	}
	return false
}

func (a *AccessTokenPool) SetCanUseAt(token string, canUseAt int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, v := range a.AccessTokens {
		if v.Token == token {
			v.CanUseAt = canUseAt
			break
		}
	}
}
