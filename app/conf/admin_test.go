package conf

import (
	"testing"
)

func TestParseChatGPTSessionAccount(t *testing.T) {
	data := []byte(`{
		"user": {"id":"user-example","email":"user@example.com","iat":1781363828},
		"expires":"2026-09-11T15:17:16.660Z",
		"account": {"id":"70545398-7f63-4538-84d3-a21e10864415","planType":"free"},
		"accessToken":"Bearer upstream-access-token",
		"authProvider":"openai",
		"sessionToken":"unused-session-token"
	}`)

	account, ok, err := parseChatGPTSessionAccount(data)
	if err != nil {
		t.Fatalf("parse session: %v", err)
	}
	if !ok {
		t.Fatal("expected payload to be recognized as ChatGPT session JSON")
	}
	if account.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if account.AccessToken != "upstream-access-token" {
		t.Fatalf("unexpected access token: %q", account.AccessToken)
	}
	if account.Email != "user@example.com" {
		t.Fatalf("unexpected email: %q", account.Email)
	}
	if account.AccountID != "70545398-7f63-4538-84d3-a21e10864415" {
		t.Fatalf("unexpected account id: %q", account.AccountID)
	}
	if account.Type != "free" {
		t.Fatalf("unexpected type: %q", account.Type)
	}
	if account.Expired != "2026-09-11T15:17:16.660Z" {
		t.Fatalf("unexpected expired: %q", account.Expired)
	}
	if account.LastRefresh != "2026-06-13T13:57:08Z" {
		t.Fatalf("unexpected last refresh: %q", account.LastRefresh)
	}
}

func TestMergeImportedChatGPTAccountUpdatesExistingAndKeepsLocalSettings(t *testing.T) {
	disabled := false
	existing := chatgpt{
		ID:             "local-id",
		Enabled:        &disabled,
		Priority:       7,
		AccessToken:    "old-token",
		AccountId:      "account-1",
		Proxy:          "http://127.0.0.1:7890",
		SelectedModels: []string{"gpt-4o"},
	}
	imported := chatgpt{
		ID:          "account-1",
		AccessToken: "new-token",
		AccountId:   "account-1",
		Email:       "user@example.com",
		Type:        "plus",
		Expired:     "2026-01-01T00:00:00Z",
	}

	merged := mergeImportedChatGPTAccount([]chatgpt{existing}, imported)
	if len(merged) != 1 {
		t.Fatalf("expected one account, got %d", len(merged))
	}
	if merged[0].AccessToken != "new-token" {
		t.Fatalf("expected token update, got %q", merged[0].AccessToken)
	}
	if merged[0].ID != "local-id" {
		t.Fatalf("expected selector id preserved, got %q", merged[0].ID)
	}
	if merged[0].IsEnabled() {
		t.Fatal("expected existing disabled state to be preserved")
	}
	if merged[0].Priority != 7 || merged[0].Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("expected local settings preserved, got priority=%d proxy=%q", merged[0].Priority, merged[0].Proxy)
	}
	if len(merged[0].SelectedModels) != 1 || merged[0].SelectedModels[0] != "gpt-4o" {
		t.Fatalf("expected selected models preserved, got %#v", merged[0].SelectedModels)
	}
}
