package conf

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"os"
	"strings"
	"sync"
)

const adminSessionCookie = "chat2api_admin_session"

var (
	adminSessionMu    sync.RWMutex
	adminSessionToken string
)

func AdminEnabled() bool {
	return strings.TrimSpace(os.Getenv("ADMIN_USERNAME")) != "" && strings.TrimSpace(os.Getenv("ADMIN_PASSWORD")) != ""
}

func AdminUsername() string {
	return strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
}

func ValidateAdminCredentials(username string, password string) bool {
	if !AdminEnabled() {
		return false
	}
	storedUsername := AdminUsername()
	storedPassword := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(username)), []byte(storedUsername)) != 1 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(storedPassword)) == 1
}

func NewAdminSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	token := hex.EncodeToString(sum[:])
	adminSessionMu.Lock()
	defer adminSessionMu.Unlock()
	adminSessionToken = token
	return token, nil
}

func ValidateAdminSessionToken(token string) bool {
	adminSessionMu.RLock()
	defer adminSessionMu.RUnlock()
	if adminSessionToken == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(adminSessionToken)) == 1
}

func ClearAdminSessionToken() {
	adminSessionMu.Lock()
	defer adminSessionMu.Unlock()
	adminSessionToken = ""
}

func AdminSessionCookieName() string {
	return adminSessionCookie
}
