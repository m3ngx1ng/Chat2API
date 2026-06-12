package conf

import (
	"chat2api/app/env"
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	configPathMu      sync.RWMutex
	currentConfigPath string
	adminConfigMu     sync.Mutex
)

var ErrConfigReadonly = errors.New("current runtime is not backed by a writable config file")

type AdminChatGPTAccount struct {
	IdToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
	LastRefresh  string `json:"last_refresh"`
	Email        string `json:"email"`
	Type         string `json:"type"`
	Expired      string `json:"expired"`
	Proxy        string `json:"proxy"`
}

type AdminConfigSnapshot struct {
	ConfigPath          string                `json:"config_path"`
	Writable            bool                  `json:"writable"`
	RuntimeBind         string                `json:"runtime_bind"`
	RuntimePort         uint16                `json:"runtime_port"`
	Proxy               string                `json:"proxy"`
	ChatGPTBaseURL      string                `json:"chatgpt_base_url"`
	AuthTokens          []string              `json:"auth_tokens"`
	AccessTokenPrefixes []string              `json:"access_token_prefixes"`
	ChatGPTAccounts     []AdminChatGPTAccount `json:"chatgpt_accounts"`
}

type AdminConfigUpdate struct {
	Proxy               string                `json:"proxy"`
	ChatGPTBaseURL      string                `json:"chatgpt_base_url"`
	AuthTokens          []string              `json:"auth_tokens"`
	AccessTokenPrefixes []string              `json:"access_token_prefixes"`
	ChatGPTAccounts     []AdminChatGPTAccount `json:"chatgpt_accounts"`
}

func setCurrentConfigPath(path string) {
	configPathMu.Lock()
	defer configPathMu.Unlock()
	currentConfigPath = strings.TrimSpace(path)
}

func getCurrentConfigPath() string {
	configPathMu.RLock()
	defer configPathMu.RUnlock()
	return currentConfigPath
}

func AdminSnapshot() (*AdminConfigSnapshot, error) {
	runtimeCfg := GetApp()
	persistedCfg, path, writable, err := readPersistedConfig()
	if err != nil && !errors.Is(err, ErrConfigReadonly) {
		return nil, err
	}
	if !writable {
		persistedCfg = runtimeCfg
	}
	return &AdminConfigSnapshot{
		ConfigPath:          path,
		Writable:            writable,
		RuntimeBind:         runtimeCfg.Bind,
		RuntimePort:         runtimeCfg.Port,
		Proxy:               persistedCfg.Proxy,
		ChatGPTBaseURL:      persistedCfg.ChatGPTBaseUrl,
		AuthTokens:          cloneStrings(persistedCfg.Auth.AccessTokens),
		AccessTokenPrefixes: cloneStrings(persistedCfg.Auth.AccessTokenPrefix),
		ChatGPTAccounts:     adminAccountsFromConfig(persistedCfg.ChatGPTs),
	}, nil
}

func SaveAdminConfig(input AdminConfigUpdate) error {
	adminConfigMu.Lock()
	defer adminConfigMu.Unlock()

	persistedCfg, path, writable, err := readPersistedConfig()
	if err != nil {
		return err
	}
	if !writable {
		return ErrConfigReadonly
	}

	persistedCfg.Proxy = strings.TrimSpace(input.Proxy)
	persistedCfg.ChatGPTBaseUrl = strings.TrimSpace(input.ChatGPTBaseURL)
	persistedCfg.Auth.AccessTokens = nonEmptyAuthTokens(input.AuthTokens)
	persistedCfg.Auth.AccessTokenPrefix = nonEmptyAccessTokenPrefixes(input.AccessTokenPrefixes)
	persistedCfg.ChatGPTs = adminAccountsToConfig(input.ChatGPTAccounts)

	if err := saveFullConfig(path, persistedCfg); err != nil {
		return err
	}

	runtimeCfg := persistedCfg
	applyRuntimeOverrides(&runtimeCfg)
	normalizeConfig(&runtimeCfg)
	setApp(runtimeCfg)
	logCurrentAuth(context.Background(), runtimeCfg)
	return nil
}

func readPersistedConfig() (app, string, bool, error) {
	path := getCurrentConfigPath()
	if path == "" {
		return GetApp(), "", false, ErrConfigReadonly
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return app{}, path, true, err
	}
	next := defaultGeneratedApp(env.Curr)
	if err := yaml.Unmarshal(data, &next); err != nil {
		return app{}, path, true, err
	}
	return next, path, true, nil
}

func saveFullConfig(path string, cfg app) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func adminAccountsFromConfig(accounts []chatgpt) []AdminChatGPTAccount {
	items := make([]AdminChatGPTAccount, 0, len(accounts))
	for _, account := range accounts {
		if strings.TrimSpace(account.AccessToken) == "" &&
			strings.TrimSpace(account.Proxy) == "" &&
			strings.TrimSpace(account.Email) == "" &&
			strings.TrimSpace(account.Type) == "" &&
			strings.TrimSpace(account.IdToken) == "" &&
			strings.TrimSpace(account.RefreshToken) == "" &&
			strings.TrimSpace(account.AccountId) == "" &&
			strings.TrimSpace(account.LastRefresh) == "" &&
			strings.TrimSpace(account.Expired) == "" {
			continue
		}
		items = append(items, AdminChatGPTAccount{
			IdToken:      strings.TrimSpace(account.IdToken),
			AccessToken:  normalizeAuthToken(account.AccessToken),
			RefreshToken: strings.TrimSpace(account.RefreshToken),
			AccountID:    strings.TrimSpace(account.AccountId),
			LastRefresh:  strings.TrimSpace(account.LastRefresh),
			Email:        strings.TrimSpace(account.Email),
			Type:         strings.TrimSpace(account.Type),
			Expired:      strings.TrimSpace(account.Expired),
			Proxy:        strings.TrimSpace(account.Proxy),
		})
	}
	return items
}

func adminAccountsToConfig(accounts []AdminChatGPTAccount) []chatgpt {
	items := make([]chatgpt, 0, len(accounts))
	for _, account := range accounts {
		normalized := chatgpt{
			IdToken:      strings.TrimSpace(account.IdToken),
			AccessToken:  normalizeAuthToken(account.AccessToken),
			RefreshToken: strings.TrimSpace(account.RefreshToken),
			AccountId:    strings.TrimSpace(account.AccountID),
			LastRefresh:  strings.TrimSpace(account.LastRefresh),
			Email:        strings.TrimSpace(account.Email),
			Type:         strings.TrimSpace(account.Type),
			Expired:      strings.TrimSpace(account.Expired),
			Proxy:        strings.TrimSpace(account.Proxy),
		}
		if normalized.AccessToken == "" &&
			normalized.Proxy == "" &&
			normalized.Email == "" &&
			normalized.Type == "" &&
			normalized.IdToken == "" &&
			normalized.RefreshToken == "" &&
			normalized.AccountId == "" &&
			normalized.LastRefresh == "" &&
			normalized.Expired == "" {
			continue
		}
		items = append(items, normalized)
	}
	return items
}

func cloneStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
