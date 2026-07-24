// Package subauth stores and refreshes consumer subscription OAuth credentials
// (ChatGPT/Codex, Claude, SuperGrok) for the gateway process.
//
// Tokens live in a mode-0600 JSON file under the user config dir. Never log
// access or refresh tokens.
package subauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Provider IDs in the credential store.
const (
	ProviderChatGPT = "chatgpt"
	ProviderClaude  = "claude"
	ProviderGrok    = "grok"
)

// Credential is one provider's OAuth (or long-lived setup) token set.
type Credential struct {
	Provider     string    `json:"provider"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	TokenURL     string    `json:"token_url,omitempty"`
	// AccountID is optional ChatGPT account id extracted from the access token.
	AccountID string `json:"account_id,omitempty"`
	// Source describes how the credential was obtained (oauth_pkce, device_code, setup_token, import).
	Source    string    `json:"source,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Store is the on-disk credentials map.
type Store struct {
	Version     int                   `json:"version"`
	Credentials map[string]Credential `json:"credentials"`
}

const storeVersion = 1

// DefaultPath returns ~/.config/inja-gateway/credentials.json (or $INJA_GATEWAY_AUTH_FILE).
func DefaultPath() (string, error) {
	if p := os.Getenv("INJA_GATEWAY_AUTH_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "inja-gateway", "credentials.json"), nil
}

// ResolvePath returns INJA_GATEWAY_AUTH_FILE if set, otherwise DefaultPath.
func ResolvePath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("INJA_GATEWAY_AUTH_FILE")); p != "" {
		return p, nil
	}
	return DefaultPath()
}

// HasUsableCredential reports whether the store has a credential that can
// still produce an access token (live access, or refresh_token present).
// Expired access without refresh is unusable.
func HasUsableCredential(path, provider string) bool {
	c, ok := LoadCredential(path, provider)
	if !ok {
		return false
	}
	return c.Usable(time.Now())
}

// LoadCredential loads one provider from path (missing file → false).
func LoadCredential(path, provider string) (Credential, bool) {
	if path == "" || provider == "" {
		return Credential{}, false
	}
	store, err := Load(path)
	if err != nil {
		return Credential{}, false
	}
	return store.Get(provider)
}

// Usable reports whether this credential can still authorize requests.
func (c Credential) Usable(now time.Time) bool {
	if c.AccessToken == "" && c.RefreshToken == "" {
		return false
	}
	// Live access token (zero expiry = long-lived / unknown).
	if c.AccessToken != "" && (c.Expiry.IsZero() || now.Before(c.Expiry)) {
		return true
	}
	// Can refresh.
	return c.RefreshToken != ""
}

// Load reads the store from path. Missing file → empty store.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{Version: storeVersion, Credentials: map[string]Credential{}}, nil
		}
		return nil, fmt.Errorf("subauth: read %s: %w", path, err)
	}
	var s Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("subauth: parse %s: %w", path, err)
	}
	if s.Credentials == nil {
		s.Credentials = map[string]Credential{}
	}
	if s.Version == 0 {
		s.Version = storeVersion
	}
	return &s, nil
}

// Save writes the store with 0600 permissions (dir 0700).
func (s *Store) Save(path string) error {
	if s == nil {
		return fmt.Errorf("subauth: nil store")
	}
	if s.Credentials == nil {
		s.Credentials = map[string]Credential{}
	}
	s.Version = storeVersion
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("subauth: mkdir: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("subauth: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("subauth: rename: %w", err)
	}
	return nil
}

// Get returns a credential copy.
func (s *Store) Get(provider string) (Credential, bool) {
	if s == nil || s.Credentials == nil {
		return Credential{}, false
	}
	c, ok := s.Credentials[provider]
	return c, ok
}

// Put stores a credential (updates UpdatedAt).
func (s *Store) Put(c Credential) {
	if s.Credentials == nil {
		s.Credentials = map[string]Credential{}
	}
	c.UpdatedAt = time.Now().UTC()
	s.Credentials[c.Provider] = c
}

// Delete removes a provider credential.
func (s *Store) Delete(provider string) {
	if s == nil || s.Credentials == nil {
		return
	}
	delete(s.Credentials, provider)
}

// ValidProviders lists known provider ids.
func ValidProviders() []string {
	return []string{ProviderChatGPT, ProviderClaude, ProviderGrok}
}

// IsKnownProvider reports whether id is a supported subscription provider.
func IsKnownProvider(id string) bool {
	switch id {
	case ProviderChatGPT, ProviderClaude, ProviderGrok:
		return true
	default:
		return false
	}
}

// FileMutex serializes refresh writes for a store path (process-local).
type FileMutex struct {
	mu sync.Mutex
}

func (f *FileMutex) Lock()   { f.mu.Lock() }
func (f *FileMutex) Unlock() { f.mu.Unlock() }
