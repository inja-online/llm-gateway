package dashboard

import (
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

// ProfileDTO is the operator-facing credential view. Never add
// access_token, refresh_token, or client_secret tags here.
type ProfileDTO struct {
	Provider      string     `json:"provider"`
	AccountID     string     `json:"account_id"`
	Source        string     `json:"source"`
	Expiry        *time.Time `json:"expiry"`
	Usable        bool       `json:"usable"`
	HasRefresh    bool       `json:"has_refresh"`
	HasAccess     bool       `json:"has_access"`
	AccessState   string     `json:"access_state"`
	CooldownUntil *time.Time `json:"cooldown_until"`
	Disabled      bool       `json:"disabled"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

func toDTO(a subauth.Account, now time.Time) ProfileDTO {
	p := ProfileDTO{
		Provider:    a.Provider,
		AccountID:   a.ID,
		Source:      a.Source,
		Usable:      a.Usable(now),
		HasRefresh:  a.RefreshToken != "",
		HasAccess:   a.AccessToken != "",
		AccessState: accessState(a.Credential, now),
		Disabled:    a.Disabled,
	}
	if !a.Expiry.IsZero() {
		t := a.Expiry.UTC()
		p.Expiry = &t
	}
	if !a.UpdatedAt.IsZero() {
		t := a.UpdatedAt.UTC()
		p.UpdatedAt = &t
	}
	if !a.CooldownUntil.IsZero() {
		t := a.CooldownUntil.UTC()
		p.CooldownUntil = &t
	}
	return p
}

func missingDTO(provider string) ProfileDTO {
	return ProfileDTO{Provider: provider, AccessState: "missing"}
}

func accessState(c subauth.Credential, now time.Time) string {
	if c.AccessToken == "" {
		return "empty"
	}
	if !c.Expiry.IsZero() && now.After(c.Expiry) {
		return "expired"
	}
	return "present"
}
