package conf

import (
	"chat2api/app/env"
	"chat2api/app/token_pool"
	"chat2api/pkg/logx"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Loader struct {
	path        string
	sha256      string
	watchCancel context.CancelFunc
	lock        sync.RWMutex
	loadFn      func(data []byte) error
	ctx         context.Context
}

func (l *Loader) init() error {
	sha256hex, err := l.configSHA256()
	if err != nil {
		return err
	}
	l.sha256 = sha256hex
	ctx, cancel := context.WithCancel(context.TODO())
	l.watchCancel = cancel
	go l.watch(ctx)
	return nil
}

func (l *Loader) executeLoader() error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	file, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	return l.loadFn(file)
}

func (l *Loader) watch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		func() {
			sha256hex, err := l.configSHA256()
			if err != nil {
				logx.WithContext(l.ctx).Errorf("watch config file error: %+v", err)
				return
			}
			if sha256hex != l.sha256 {
				logx.WithContext(l.ctx).Infof("config file changed, reload config, last sha256: %s, new sha256: %s", l.sha256, sha256hex)
				if err := l.executeLoader(); err != nil {
					logx.WithContext(l.ctx).Errorf("execute config loader error with new sha256: %s: %+v, config digest will not be changed until all loaders are succeeded", sha256hex, err)
					return
				}
				l.sha256 = sha256hex
			}
		}()
	}
}

func (l *Loader) sumSHA256(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}

func (l *Loader) configSHA256() (string, error) {
	configData, err := os.ReadFile(l.path)
	if err != nil {
		return "", err
	}
	return l.sumSHA256(configData), nil
}

func (l *Loader) Close() {
	l.watchCancel()
}

func NewLoader(ctx context.Context, path string, loadFn func([]byte) error) (*Loader, error) {
	logx.WithContext(ctx).Infof("load config %v", path)
	l := &Loader{path: path, loadFn: loadFn, ctx: ctx}
	if err := l.init(); err != nil {
		return nil, err
	}

	file, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}
	if err = l.loadFn(file); err != nil {
		return nil, err
	}
	if sha256hex, err := l.configSHA256(); err == nil {
		l.sha256 = sha256hex
	}

	return l, nil
}

var (
	appMu sync.RWMutex
	App   = defaultApp()
)

func GetApp() app {
	appMu.RLock()
	defer appMu.RUnlock()
	return App
}

func setApp(next app) {
	appMu.Lock()
	defer appMu.Unlock()
	App = next
}

func defaultApp() app {
	return app{
		Bind: "0.0.0.0",
		Port: 3040,
	}
}

func defaultGeneratedApp(curr env.Env) app {
	bind := "127.0.0.1"
	logLevel := "debug"
	if curr == env.PROD {
		bind = "0.0.0.0"
		logLevel = "info"
	}
	return app{
		LogLevel:       logLevel,
		LogPath:        "logs",
		LogFile:        "",
		Bind:           bind,
		Port:           3040,
		Auth:           auth{AccessTokens: []string{}, AccessTokenPrefix: []string{}},
		Proxy:          "",
		ChatGPTBaseUrl: "https://chatgpt.com",
		AccountRouting: accountRouting{Mode: AccountRoutingModeRoundRobin},
		ChatGPTs:       []chatgpt{},
	}
}

func ensureConfigFile(ctx context.Context, path string, curr env.Env) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(defaultGeneratedApp(curr))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	logx.WithContext(ctx).Infof("config file not found, generated default config: %s", path)
	return nil
}

func resolveConfigPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("VERCEL_CONFIG_FILE")); path != "" {
		if filepath.IsAbs(path) {
			return path, nil
		}
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(wd, path), nil
	}

	configFile := fmt.Sprintf("app.%s.yaml", env.Curr)
	for _, dir := range []string{
		strings.TrimSpace(os.Getenv("CONFIG_DIR")),
		strings.TrimSpace(os.Getenv("RENDER_DISK_MOUNT_PATH")),
	} {
		if dir == "" {
			continue
		}
		if filepath.IsAbs(dir) {
			return filepath.Join(dir, configFile), nil
		}
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(wd, dir, configFile), nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "conf", configFile), nil
}

func ensureAuthTokens(path string, cfg *app) error {
	if len(nonEmptyAuthTokens(cfg.Auth.AccessTokens)) > 0 {
		cfg.Auth.AccessTokens = nonEmptyAuthTokens(cfg.Auth.AccessTokens)
		return nil
	}
	token, err := newAuthToken()
	if err != nil {
		return err
	}
	cfg.Auth.AccessTokens = []string{token}
	return saveAuthTokens(path, cfg.Auth.AccessTokens)
}

func normalizeConfig(cfg *app) {
	cfg.Auth.AccessTokens = nonEmptyAuthTokens(cfg.Auth.AccessTokens)
	for i, token := range cfg.Auth.AccessTokens {
		cfg.Auth.AccessTokens[i] = normalizeAuthToken(token)
	}
	cfg.Auth.AccessTokenPrefix = nonEmptyAccessTokenPrefixes(cfg.Auth.AccessTokenPrefix)
	cfg.AccountRouting.Mode = normalizeAccountRoutingMode(cfg.AccountRouting.Mode)
	cfg.AccountRouting.SelectedAccount = strings.TrimSpace(cfg.AccountRouting.SelectedAccount)
	pool := token_pool.GetAccessTokenPool()
	pool.Reset()
	for _, account := range selectedChatGPTAccounts(*cfg) {
		token := strings.TrimSpace(account.AccessToken)
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" {
			continue
		}
		pool.AddAccessToken(&token_pool.AccessToken{
			Token:     "Bearer " + token,
			ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
			Proxy:     strings.TrimSpace(account.Proxy),
			Models:    normalizeModelNames(account.SelectedModels),
		})
	}
}

func maskedAuthTokens(tokens []string) []string {
	masked := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = normalizeAuthToken(token)
		if token == "" {
			continue
		}
		masked = append(masked, maskToken(token))
	}
	return masked
}

func maskedAccessTokenPrefixes(prefixes []string) []string {
	masked := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		masked = append(masked, maskToken(prefix))
	}
	return masked
}

func logCurrentAuth(ctx context.Context, cfg app) {
	logx.WithContext(ctx).Infof(
		"current auth tokens: count=%d masked=%s, access_token_prefix: count=%d masked=%s",
		len(cfg.Auth.AccessTokens),
		strings.Join(maskedAuthTokens(cfg.Auth.AccessTokens), ", "),
		len(cfg.Auth.AccessTokenPrefix),
		strings.Join(maskedAccessTokenPrefixes(cfg.Auth.AccessTokenPrefix), ", "),
	)
}

func maskToken(token string) string {
	if len(token) <= 10 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-4:]
}

func nonEmptyAuthTokens(tokens []string) []string {
	normalized := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = normalizeAuthToken(token)
		if token != "" {
			normalized = append(normalized, token)
		}
	}
	return normalized
}

func nonEmptyAccessTokenPrefixes(prefixes []string) []string {
	normalized := make([]string, 0, len(prefixes))
	seen := make(map[string]struct{}, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		normalized = append(normalized, prefix)
	}
	return normalized
}

func normalizeAuthToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	return strings.TrimSpace(token)
}

func newAuthToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(b[:]), nil
}

func NewAuthToken() (string, error) {
	return newAuthToken()
}

func saveAuthTokens(path string, tokens []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	root := mappingRoot(&doc)
	authNode := ensureMappingChild(root, "auth")
	accessTokens := &yaml.Node{Kind: yaml.SequenceNode}
	for _, token := range tokens {
		token = normalizeAuthToken(token)
		if token == "" {
			continue
		}
		accessTokens.Content = append(accessTokens.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: token,
		})
	}
	setMappingChild(authNode, "access_tokens", accessTokens)
	data, err = yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func mappingRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		if doc.Content[0].Kind != yaml.MappingNode {
			doc.Content[0].Kind = yaml.MappingNode
		}
		return doc.Content[0]
	}
	doc.Kind = yaml.DocumentNode
	doc.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	return doc.Content[0]
}

func ensureMappingChild(root *yaml.Node, key string) *yaml.Node {
	if value := mappingChild(root, key); value != nil {
		if value.Kind != yaml.MappingNode {
			value.Kind = yaml.MappingNode
			value.Content = nil
		}
		return value
	}
	value := &yaml.Node{Kind: yaml.MappingNode}
	setMappingChild(root, key, value)
	return value
}

func mappingChild(root *yaml.Node, key string) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

func setMappingChild(root *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			root.Content[i+1] = value
			return
		}
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		value,
	)
}

func Init(ctx context.Context) func(context.Context) {
	path, err := resolveConfigPath()
	if err != nil {
		logx.WithContext(ctx).Fatalf("resolve config path failed: %+v", err)
	}
	setCurrentConfigPath(path)
	if err := ensureConfigFile(ctx, path, env.Curr); err != nil {
		logx.WithContext(ctx).Fatalf("generate config failed: %+v", err)
	}
	loader, err := NewLoader(ctx, path, func(data []byte) error {
		next := defaultApp()
		if err := yaml.Unmarshal(data, &next); err != nil {
			return err
		}
		if err := ensureAuthTokens(path, &next); err != nil {
			return err
		}
		applyRuntimeOverrides(&next)
		normalizeConfig(&next)
		if err := validateAccountRouting(next); err != nil {
			return err
		}
		if err := logx.Configure(next.LogLevel, next.LogPath, next.LogFile); err != nil {
			return err
		}
		setApp(next)
		logCurrentAuth(ctx, next)
		return nil
	})
	if err != nil {
		logx.WithContext(ctx).Fatalf("load config failed: %+v", err)
	}

	return func(context.Context) {
		if loader != nil {
			loader.Close()
		}
		logx.CloseOutput()
	}
}

func InitServerless(ctx context.Context) func(context.Context) {
	next := defaultGeneratedApp(env.Curr)
	setCurrentConfigPath("")
	if path := strings.TrimSpace(os.Getenv("VERCEL_CONFIG_FILE")); path != "" {
		resolvedPath, err := resolveConfigPath()
		if err != nil {
			logx.WithContext(ctx).Fatalf("resolve config path failed: %+v", err)
		}
		path = resolvedPath
		setCurrentConfigPath(path)
		data, err := os.ReadFile(path)
		if err != nil {
			logx.WithContext(ctx).Fatalf("load config failed: %+v", err)
		}
		next = defaultApp()
		if err := yaml.Unmarshal(data, &next); err != nil {
			logx.WithContext(ctx).Fatalf("parse config failed: %+v", err)
		}
	}
	applyEnvOverrides(&next)
	applyRuntimeOverrides(&next)
	next.Auth.AccessTokens = nonEmptyAuthTokens(next.Auth.AccessTokens)
	normalizeConfig(&next)
	if err := validateAccountRouting(next); err != nil {
		logx.WithContext(ctx).Fatalf("validate config failed: %+v", err)
	}
	if err := logx.Configure(next.LogLevel, next.LogPath, ""); err != nil {
		logx.WithContext(ctx).Fatalf("configure log failed: %+v", err)
	}
	setApp(next)
	logCurrentAuth(ctx, next)
	return func(context.Context) {
		logx.CloseOutput()
	}
}

func applyEnvOverrides(cfg *app) {
	if value := strings.TrimSpace(os.Getenv("AUTH_TOKENS")); value != "" {
		cfg.Auth.AccessTokens = splitEnvList(value)
	}
	if value := strings.TrimSpace(os.Getenv("ACCESS_TOKENS")); value != "" {
		cfg.Auth.AccessTokens = splitEnvList(value)
	}
	if value := strings.TrimSpace(os.Getenv("ACCESS_TOKEN_PREFIX")); value != "" {
		cfg.Auth.AccessTokenPrefix = splitEnvList(value)
	}
	if value := strings.TrimSpace(os.Getenv("ACCESS_TOKEN_PREFIXES")); value != "" {
		cfg.Auth.AccessTokenPrefix = splitEnvList(value)
	}
	if value := strings.TrimSpace(os.Getenv("CHATGPT_ACCESS_TOKENS")); value != "" {
		for _, token := range splitEnvList(value) {
			cfg.ChatGPTs = append(cfg.ChatGPTs, chatgpt{AccessToken: token})
		}
	}
	if value := strings.TrimSpace(os.Getenv("PROXY")); value != "" {
		cfg.Proxy = value
	}
	if value := strings.TrimSpace(os.Getenv("CHATGPT_BASE_URL")); value != "" {
		cfg.ChatGPTBaseUrl = value
	}
	if value := strings.TrimSpace(os.Getenv("LOG_LEVEL")); value != "" {
		cfg.LogLevel = value
	}
}

func selectedChatGPTAccounts(cfg app) []chatgpt {
	type indexedAccount struct {
		order   int
		account chatgpt
	}

	accounts := make([]indexedAccount, 0, len(cfg.ChatGPTs))
	for index, account := range cfg.ChatGPTs {
		if !account.IsEnabled() {
			continue
		}
		account.AccessToken = normalizedAccessToken(account.AccessToken)
		if account.AccessToken == "" {
			continue
		}
		if strings.TrimSpace(account.ID) == "" {
			account.ID = account.Selector()
		}
		accounts = append(accounts, indexedAccount{order: index, account: account})
	}

	if cfg.ChatGPTRoutingMode() == AccountRoutingModeSingle {
		selector := strings.TrimSpace(cfg.AccountRouting.SelectedAccount)
		for _, item := range accounts {
			if item.account.MatchesSelector(selector) {
				return []chatgpt{item.account}
			}
		}
		return []chatgpt{}
	}

	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].account.Priority != accounts[j].account.Priority {
			return accounts[i].account.Priority < accounts[j].account.Priority
		}
		return accounts[i].order < accounts[j].order
	})

	out := make([]chatgpt, 0, len(accounts))
	for _, item := range accounts {
		out = append(out, item.account)
	}
	return out
}

func validateAccountRouting(cfg app) error {
	if cfg.ChatGPTRoutingMode() != AccountRoutingModeSingle {
		return nil
	}
	selector := strings.TrimSpace(cfg.AccountRouting.SelectedAccount)
	if selector == "" {
		return fmt.Errorf("single account mode requires selected_account")
	}
	for _, account := range cfg.ChatGPTs {
		if !account.IsEnabled() {
			continue
		}
		if normalizedAccessToken(account.AccessToken) == "" {
			continue
		}
		if account.MatchesSelector(selector) {
			return nil
		}
	}
	return fmt.Errorf("selected_account does not match any enabled chatgpt account")
}

func applyRuntimeOverrides(cfg *app) {
	if value := strings.TrimSpace(os.Getenv("BIND")); value != "" {
		cfg.Bind = value
	}
	if value := strings.TrimSpace(os.Getenv("HOST")); value != "" {
		cfg.Bind = value
	}
	if value := strings.TrimSpace(os.Getenv("PORT")); value != "" {
		port, err := strconv.ParseUint(value, 10, 16)
		if err == nil && port > 0 {
			cfg.Port = uint16(port)
			if cfg.Bind == "" || cfg.Bind == "127.0.0.1" || strings.EqualFold(cfg.Bind, "localhost") {
				cfg.Bind = "0.0.0.0"
			}
		}
	}
}

func splitEnvList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
