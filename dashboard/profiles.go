package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/inja-online/llm-gateway/subauth"
)

func (s *Server) handleProfiles(w http.ResponseWriter, _ *http.Request) {
	store, err := subauth.Load(s.opts.AuthPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
		return
	}
	now := time.Now()
	var profiles []ProfileDTO
	for _, id := range subauth.ValidProviders() {
		accts := store.ListAccounts(id)
		if len(accts) == 0 {
			profiles = append(profiles, missingDTO(id))
			continue
		}
		for _, a := range accts {
			if a.Provider == "" {
				a.Provider = id
			}
			profiles = append(profiles, toDTO(a, now))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider != "all" && !subauth.IsKnownProvider(provider) {
		writeError(w, http.StatusNotFound, "invalid_request_error", "", "unknown provider")
		return
	}
	store, err := subauth.Load(s.opts.AuthPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
		return
	}
	account := r.URL.Query().Get("account")
	if provider == "all" {
		for _, id := range subauth.ValidProviders() {
			store.Delete(id)
			if store.Pool != nil {
				delete(store.Pool, id)
			}
		}
	} else if account == "" {
		store.Delete(provider)
	} else {
		removePoolAccount(store, provider, account)
	}
	if err := store.Save(s.opts.AuthPath); err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDisable(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !subauth.IsKnownProvider(provider) {
		writeError(w, http.StatusNotFound, "invalid_request_error", "", "unknown provider")
		return
	}
	var body struct {
		Disabled bool   `json:"disabled"`
		Account  string `json:"account"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "", "bad json")
		return
	}
	store, err := subauth.Load(s.opts.AuthPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
		return
	}
	var found *subauth.Account
	for _, a := range store.ListAccounts(provider) {
		if a.ID == body.Account {
			a := a
			found = &a
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "invalid_request_error", "", "unknown account")
		return
	}
	found.Provider = provider
	found.Disabled = body.Disabled
	store.PutAccount(*found)
	if err := store.Save(s.opts.AuthPath); err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", "", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func removePoolAccount(store *subauth.Store, provider, id string) {
	if store.Pool == nil {
		return
	}
	list := store.Pool[provider]
	out := list[:0]
	for _, a := range list {
		if a.ID != id {
			out = append(out, a)
		}
	}
	store.Pool[provider] = out
}
