package conf

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	AccountRoutingModeRoundRobin = "round_robin"
	AccountRoutingModeSingle     = "single"
)

type app struct {
	LogLevel       string         `yaml:"log_level"`
	LogPath        string         `yaml:"log_path"`
	LogFile        string         `yaml:"log_file"`
	Bind           string         `yaml:"bind"`
	Port           uint16         `yaml:"port"`
	Auth           auth           `yaml:"auth"`
	Proxy          string         `yaml:"proxy"`
	ChatGPTBaseUrl string         `yaml:"chatgpt_base_url"`
	AccountRouting accountRouting `yaml:"account_routing"`
	ChatGPTs       []chatgpt      `yaml:"chatgpts"`
}

func (a app) TextAccessTokens() []string {
	tokens := make([]string, 0, len(a.ChatGPTs))
	for _, account := range a.ChatGPTs {
		if !account.IsEnabled() {
			continue
		}
		if token := normalizedAccessToken(account.AccessToken); token != "" {
			tokens = append(tokens, account.AccessToken)
		}
	}
	return tokens
}

func (a app) SummaryModels() []string {
	accounts := selectedChatGPTAccounts(a)
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, account := range accounts {
		for _, model := range normalizeModelNames(account.SelectedModels) {
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			out = append(out, model)
		}
	}
	if len(out) == 0 {
		return []string{"auto"}
	}
	return out
}

func (a app) FindChatGPTAccount(selector string) (chatgpt, int, bool) {
	selector = strings.TrimSpace(strings.TrimPrefix(selector, "Bearer "))
	if selector == "" {
		return chatgpt{}, -1, false
	}
	for index, account := range a.ChatGPTs {
		if account.MatchesSelector(selector) {
			return account, index, true
		}
	}
	return chatgpt{}, -1, false
}

func normalizeModelNames(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
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

type accountRouting struct {
	Mode            string `yaml:"mode,omitempty"`
	SelectedAccount string `yaml:"selected_account,omitempty"`
}

func normalizeAccountRoutingMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case AccountRoutingModeSingle:
		return AccountRoutingModeSingle
	default:
		return AccountRoutingModeRoundRobin
	}
}

func (a app) ChatGPTRoutingMode() string {
	return normalizeAccountRoutingMode(a.AccountRouting.Mode)
}

type auth struct {
	AccessTokens      []string `yaml:"access_tokens"`
	AccessTokenPrefix []string `yaml:"access_token_prefix"`
}

func (a app) DirectAccessToken(localToken string) (string, bool) {
	token, matched := a.matchAccessTokenPrefix(localToken)
	return token, matched && token != ""
}

func (a app) DirectAccessTokenPrefixMatched(localToken string) bool {
	_, matched := a.matchAccessTokenPrefix(localToken)
	return matched
}

func (a app) HasAccessTokenPrefix() bool {
	return len(a.Auth.AccessTokenPrefix) > 0
}

func (a app) matchAccessTokenPrefix(localToken string) (string, bool) {
	localToken = strings.TrimSpace(localToken)
	for _, prefix := range a.Auth.AccessTokenPrefix {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || !strings.HasPrefix(localToken, prefix) {
			continue
		}
		token := strings.TrimSpace(strings.TrimPrefix(localToken, prefix))
		return token, true
	}
	return "", false
}

type chatgpt struct {
	ID           string `yaml:"id,omitempty"`
	Enabled      *bool  `yaml:"enabled,omitempty"`
	Priority     int    `yaml:"priority,omitempty"`
	IdToken      string `yaml:"id_token"`
	AccessToken  string `yaml:"access_token"`
	RefreshToken string `yaml:"refresh_token"`
	AccountId    string `yaml:"account_id"`
	LastRefresh  string `yaml:"last_refresh"`
	Email        string `yaml:"email"`
	Type         string `yaml:"type"`
	Expired      string `yaml:"expired"`
	Proxy        string `yaml:"proxy"`
	AvailableModels []string `yaml:"available_models,omitempty"`
	SelectedModels  []string `yaml:"selected_models,omitempty"`
}

func (c chatgpt) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func (c chatgpt) Selector() string {
	if id := strings.TrimSpace(c.ID); id != "" {
		return id
	}
	seed := strings.Join([]string{
		normalizedAccessToken(c.AccessToken),
		strings.TrimSpace(c.Email),
		strings.TrimSpace(c.AccountId),
		strings.TrimSpace(c.Type),
		strings.TrimSpace(c.Proxy),
	}, "|")
	if seed == "||||" {
		return ""
	}
	sum := sha256.Sum256([]byte(seed))
	return "acc_" + hex.EncodeToString(sum[:])[:12]
}

func (c chatgpt) MatchesSelector(selector string) bool {
	selector = strings.TrimSpace(strings.TrimPrefix(selector, "Bearer "))
	if selector == "" {
		return false
	}
	if strings.EqualFold(c.Selector(), selector) {
		return true
	}
	if email := strings.TrimSpace(c.Email); email != "" && strings.EqualFold(email, selector) {
		return true
	}
	if accountID := strings.TrimSpace(c.AccountId); accountID != "" && accountID == selector {
		return true
	}
	return normalizedAccessToken(c.AccessToken) == selector
}

func normalizedAccessToken(token string) string {
	return strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
}
