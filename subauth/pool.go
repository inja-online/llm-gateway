package subauth

import (
	"sync"
	"time"
)

// Account extends a credential with a stable slot id for multi-account pools.
// Credential fields are embedded so JSON flattens access_token etc. at the top level.
type Account struct {
	ID string `json:"id,omitempty"` // optional label, e.g. "work"
	Credential
	// CooldownUntil skips this account until this time (quota / 429).
	CooldownUntil time.Time `json:"cooldown_until,omitempty"`
	Disabled     bool      `json:"disabled,omitempty"`
}

// PoolState is process-local round-robin + cooldown for a provider.
type PoolState struct {
	mu    sync.Mutex
	rr    map[string]int       // provider → next index
	until map[string]time.Time // account key → cooldown
}

// Global process pool (per auth file is fine for personal use; single process).
var defaultPool = &PoolState{
	rr:    map[string]int{},
	until: map[string]time.Time{},
}

func accountKey(provider, id string) string {
	if id == "" {
		return provider
	}
	return provider + ":" + id
}

// ListAccounts returns all credentials for a provider: primary map entry plus pool[].
func (s *Store) ListAccounts(provider string) []Account {
	if s == nil {
		return nil
	}
	var out []Account
	if c, ok := s.Get(provider); ok && (c.AccessToken != "" || c.RefreshToken != "") {
		out = append(out, Account{ID: "", Credential: c})
	}
	if s.Pool != nil {
		for _, a := range s.Pool[provider] {
			if a.Provider == "" {
				a.Provider = provider
			}
			if a.AccessToken != "" || a.RefreshToken != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

// PutAccount stores or updates an account. Empty ID replaces primary credential.
// Non-empty ID is stored in Pool[provider].
func (s *Store) PutAccount(a Account) {
	if s.Credentials == nil {
		s.Credentials = map[string]Credential{}
	}
	if a.Provider == "" {
		return
	}
	a.UpdatedAt = time.Now().UTC()
	if a.ID == "" {
		s.Credentials[a.Provider] = a.Credential
		return
	}
	if s.Pool == nil {
		s.Pool = map[string][]Account{}
	}
	list := s.Pool[a.Provider]
	found := false
	for i := range list {
		if list[i].ID == a.ID {
			list[i] = a
			found = true
			break
		}
	}
	if !found {
		list = append(list, a)
	}
	s.Pool[a.Provider] = list
}

// PickAccount selects a usable account for provider (round-robin, skip cooldown/disabled).
func (s *Store) PickAccount(provider string, now time.Time) (Account, bool) {
	accounts := s.ListAccounts(provider)
	if len(accounts) == 0 {
		return Account{}, false
	}
	defaultPool.mu.Lock()
	defer defaultPool.mu.Unlock()

	start := defaultPool.rr[provider] % len(accounts)
	for i := 0; i < len(accounts); i++ {
		idx := (start + i) % len(accounts)
		a := accounts[idx]
		if a.Disabled {
			continue
		}
		key := accountKey(provider, a.ID)
		if until, ok := defaultPool.until[key]; ok && now.Before(until) {
			continue
		}
		if a.CooldownUntil.After(now) {
			continue
		}
		if !a.Usable(now) {
			continue
		}
		defaultPool.rr[provider] = (idx + 1) % len(accounts)
		return a, true
	}
	// All cooling down or unusable — return first usable ignoring cooldown (caller may refresh).
	for _, a := range accounts {
		if !a.Disabled && a.Usable(now) {
			return a, true
		}
	}
	return Account{}, false
}

// MarkCooldown marks an account unavailable for d (e.g. after 429).
func MarkCooldown(provider, accountID string, d time.Duration) {
	if d <= 0 {
		d = 5 * time.Minute
	}
	defaultPool.mu.Lock()
	defaultPool.until[accountKey(provider, accountID)] = time.Now().Add(d)
	defaultPool.mu.Unlock()
}

// ClearCooldown removes cooldown for an account.
func ClearCooldown(provider, accountID string) {
	defaultPool.mu.Lock()
	delete(defaultPool.until, accountKey(provider, accountID))
	defaultPool.mu.Unlock()
}

// CountUsableAccounts returns how many pool slots can authorize now.
func (s *Store) CountUsableAccounts(provider string, now time.Time) int {
	n := 0
	for _, a := range s.ListAccounts(provider) {
		if !a.Disabled && a.Usable(now) {
			n++
		}
	}
	return n
}
