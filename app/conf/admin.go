package conf

import (
	"chat2api/app/env"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	configPathMu      sync.RWMutex
	currentConfigPath string
	adminConfigMu     sync.Mutex
)

var ErrConfigReadonly = errors.New("current runtime is not backed by a writable config file")

type AdminChatGPTAccount struct {
	ID              string   `json:"id"`
	Enabled         bool     `json:"enabled"`
	Priority        int      `json:"priority"`
	AvailableModels []string `json:"available_models"`
	SelectedModels  []string `json:"selected_models"`
	IdToken         string   `json:"id_token"`
	AccessToken     string   `json:"access_token"`
	RefreshToken    string   `json:"refresh_token"`
	AccountID       string   `json:"account_id"`
	LastRefresh     string   `json:"last_refresh"`
	Email           string   `json:"email"`
	Type            string   `json:"type"`
	Expired         string   `json:"expired"`
	Proxy           string   `json:"proxy"`
}

type AdminConfigSnapshot struct {
	ConfigPath          string                `json:"config_path"`
	Writable            bool                  `json:"writable"`
	RuntimeBind         string                `json:"runtime_bind"`
	RuntimePort         uint16                `json:"runtime_port"`
	Proxy               string                `json:"proxy"`
	ChatGPTBaseURL      string                `json:"chatgpt_base_url"`
	AccountRoutingMode  string                `json:"account_routing_mode"`
	SelectedAccount     string                `json:"selected_account"`
	SummaryModels       []string              `json:"summary_models"`
	AuthTokens          []string              `json:"auth_tokens"`
	AccessTokenPrefixes []string              `json:"access_token_prefixes"`
	ChatGPTAccounts     []AdminChatGPTAccount `json:"chatgpt_accounts"`
}

type AdminConfigUpdate struct {
	Proxy               string                `json:"proxy"`
	ChatGPTBaseURL      string                `json:"chatgpt_base_url"`
	AccountRoutingMode  string                `json:"account_routing_mode"`
	SelectedAccount     string                `json:"selected_account"`
	AuthTokens          []string              `json:"auth_tokens"`
	AccessTokenPrefixes []string              `json:"access_token_prefixes"`
	ChatGPTAccounts     []AdminChatGPTAccount `json:"chatgpt_accounts"`
}

type chatGPTSessionImport struct {
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		IAT   int64  `json:"iat"`
	} `json:"user"`
	Expires string `json:"expires"`
	Account struct {
		ID       string `json:"id"`
		PlanType string `json:"planType"`
	} `json:"account"`
	AccessToken  string `json:"accessToken"`
	AuthProvider string `json:"authProvider"`
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
		AccountRoutingMode:  persistedCfg.ChatGPTRoutingMode(),
		SelectedAccount:     strings.TrimSpace(persistedCfg.AccountRouting.SelectedAccount),
		SummaryModels:       persistedCfg.SummaryModels(),
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
	persistedCfg.AccountRouting.Mode = normalizeAccountRoutingMode(input.AccountRoutingMode)
	persistedCfg.AccountRouting.SelectedAccount = strings.TrimSpace(input.SelectedAccount)
	persistedCfg.Auth.AccessTokens = nonEmptyAuthTokens(input.AuthTokens)
	persistedCfg.Auth.AccessTokenPrefix = nonEmptyAccessTokenPrefixes(input.AccessTokenPrefixes)
	persistedCfg.ChatGPTs = adminAccountsToConfig(input.ChatGPTAccounts)
	normalizeConfig(&persistedCfg)

	runtimeCfg := persistedCfg
	applyRuntimeOverrides(&runtimeCfg)
	if err := validateAccountRouting(runtimeCfg); err != nil {
		return err
	}
	if err := saveFullConfig(path, persistedCfg); err != nil {
		return err
	}
	setApp(runtimeCfg)
	logCurrentAuth(context.Background(), runtimeCfg)
	return nil
}

func ExportAdminConfig() (string, []byte, error) {
	_, path, writable, err := readPersistedConfig()
	if err != nil {
		return "", nil, err
	}
	if !writable {
		return "", nil, ErrConfigReadonly
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return filepath.Base(path), data, nil
}

func ImportAdminConfig(data []byte) error {
	adminConfigMu.Lock()
	defer adminConfigMu.Unlock()

	persistedCfg, path, writable, err := readPersistedConfig()
	if err != nil {
		return err
	}
	if !writable {
		return ErrConfigReadonly
	}
	if account, ok, err := parseChatGPTSessionAccount(data); ok || err != nil {
		if err != nil {
			return err
		}
		persistedCfg.ChatGPTs = mergeImportedChatGPTAccount(persistedCfg.ChatGPTs, adminAccountToChatGPT(account))
		normalizeConfig(&persistedCfg)
		runtimeCfg := persistedCfg
		applyRuntimeOverrides(&runtimeCfg)
		if err := validateAccountRouting(runtimeCfg); err != nil {
			return err
		}
		if err := saveFullConfig(path, persistedCfg); err != nil {
			return err
		}
		setApp(runtimeCfg)
		logCurrentAuth(context.Background(), runtimeCfg)
		return nil
	}
	persistedCfg = defaultGeneratedApp(env.Curr)
	if err := yaml.Unmarshal(data, &persistedCfg); err != nil {
		return err
	}
	normalizeConfig(&persistedCfg)
	runtimeCfg := persistedCfg
	applyRuntimeOverrides(&runtimeCfg)
	if err := validateAccountRouting(runtimeCfg); err != nil {
		return err
	}
	if err := saveFullConfig(path, persistedCfg); err != nil {
		return err
	}
	setApp(runtimeCfg)
	logCurrentAuth(context.Background(), runtimeCfg)
	return nil
}

func parseChatGPTSessionAccount(data []byte) (AdminChatGPTAccount, bool, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return AdminChatGPTAccount{}, false, nil
	}
	var session chatGPTSessionImport
	if err := json.Unmarshal([]byte(trimmed), &session); err != nil {
		return AdminChatGPTAccount{}, true, err
	}
	accessToken := normalizeAuthToken(session.AccessToken)
	if accessToken == "" {
		return AdminChatGPTAccount{}, true, errors.New("json import must be a ChatGPT /api/auth/session payload with accessToken")
	}
	accountID := strings.TrimSpace(session.Account.ID)
	typeValue := strings.TrimSpace(session.Account.PlanType)
	if typeValue == "" {
		typeValue = strings.TrimSpace(session.AuthProvider)
	}
	if typeValue == "" {
		typeValue = "chatgpt-web"
	}
	account := AdminChatGPTAccount{
		ID:          firstNonEmpty(accountID, session.User.ID),
		Enabled:     true,
		AccessToken: accessToken,
		AccountID:   accountID,
		Email:       strings.TrimSpace(session.User.Email),
		Type:        typeValue,
		Expired:     strings.TrimSpace(session.Expires),
		LastRefresh: importedSessionRefreshTime(session.User.IAT),
	}
	return account, true, nil
}

func importedSessionRefreshTime(iat int64) string {
	if iat > 0 {
		return time.Unix(iat, 0).UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func adminAccountToChatGPT(account AdminChatGPTAccount) chatgpt {
	items := adminAccountsToConfig([]AdminChatGPTAccount{account})
	if len(items) == 0 {
		return chatgpt{}
	}
	return items[0]
}

func mergeImportedChatGPTAccount(accounts []chatgpt, imported chatgpt) []chatgpt {
	if normalizedAccessToken(imported.AccessToken) == "" {
		return accounts
	}
	for i := range accounts {
		if !sameChatGPTAccount(accounts[i], imported) {
			continue
		}
		accounts[i] = mergeChatGPTAccount(accounts[i], imported)
		return accounts
	}
	if imported.Priority == 0 {
		imported.Priority = len(accounts)
	}
	return append(accounts, imported)
}

func sameChatGPTAccount(a, b chatgpt) bool {
	if av, bv := strings.TrimSpace(a.AccountId), strings.TrimSpace(b.AccountId); av != "" && bv != "" && av == bv {
		return true
	}
	if av, bv := strings.TrimSpace(a.ID), strings.TrimSpace(b.ID); av != "" && bv != "" && av == bv {
		return true
	}
	if av, bv := strings.TrimSpace(a.Email), strings.TrimSpace(b.Email); av != "" && bv != "" && strings.EqualFold(av, bv) {
		return true
	}
	return normalizedAccessToken(a.AccessToken) != "" && normalizedAccessToken(a.AccessToken) == normalizedAccessToken(b.AccessToken)
}

func mergeChatGPTAccount(existing, imported chatgpt) chatgpt {
	merged := existing
	merged.ID = preferExisting(existing.ID, imported.ID)
	merged.IdToken = preferImported(imported.IdToken, existing.IdToken)
	merged.AccessToken = preferImported(imported.AccessToken, existing.AccessToken)
	merged.RefreshToken = preferImported(imported.RefreshToken, existing.RefreshToken)
	merged.AccountId = preferImported(imported.AccountId, existing.AccountId)
	merged.LastRefresh = preferImported(imported.LastRefresh, existing.LastRefresh)
	merged.Email = preferImported(imported.Email, existing.Email)
	merged.Type = preferImported(imported.Type, existing.Type)
	merged.Expired = preferImported(imported.Expired, existing.Expired)
	merged.Proxy = preferImported(imported.Proxy, existing.Proxy)
	return merged
}

func preferImported(imported, existing string) string {
	if imported = strings.TrimSpace(imported); imported != "" {
		return imported
	}
	return strings.TrimSpace(existing)
}

func preferExisting(existing, imported string) string {
	if existing = strings.TrimSpace(existing); existing != "" {
		return existing
	}
	return strings.TrimSpace(imported)
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
			ID:              account.Selector(),
			Enabled:         account.IsEnabled(),
			Priority:        account.Priority,
			AvailableModels: normalizeModelNames(account.AvailableModels),
			SelectedModels:  normalizeModelNames(account.SelectedModels),
			IdToken:         strings.TrimSpace(account.IdToken),
			AccessToken:     normalizeAuthToken(account.AccessToken),
			RefreshToken:    strings.TrimSpace(account.RefreshToken),
			AccountID:       strings.TrimSpace(account.AccountId),
			LastRefresh:     strings.TrimSpace(account.LastRefresh),
			Email:           strings.TrimSpace(account.Email),
			Type:            strings.TrimSpace(account.Type),
			Expired:         strings.TrimSpace(account.Expired),
			Proxy:           strings.TrimSpace(account.Proxy),
		})
	}
	return items
}

func adminAccountsToConfig(accounts []AdminChatGPTAccount) []chatgpt {
	items := make([]chatgpt, 0, len(accounts))
	for _, account := range accounts {
		enabled := account.Enabled
		normalized := chatgpt{
			ID:              strings.TrimSpace(account.ID),
			Enabled:         &enabled,
			Priority:        account.Priority,
			AvailableModels: normalizeModelNames(account.AvailableModels),
			SelectedModels:  normalizeModelNames(account.SelectedModels),
			IdToken:         strings.TrimSpace(account.IdToken),
			AccessToken:     normalizeAuthToken(account.AccessToken),
			RefreshToken:    strings.TrimSpace(account.RefreshToken),
			AccountId:       strings.TrimSpace(account.AccountID),
			LastRefresh:     strings.TrimSpace(account.LastRefresh),
			Email:           strings.TrimSpace(account.Email),
			Type:            strings.TrimSpace(account.Type),
			Expired:         strings.TrimSpace(account.Expired),
			Proxy:           strings.TrimSpace(account.Proxy),
		}
		if normalized.ID == "" {
			normalized.ID = normalized.Selector()
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
