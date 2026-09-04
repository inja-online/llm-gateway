package subauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartGrokDeviceAndPoll(t *testing.T) {
	var tokenHits atomic.Int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"device_authorization_endpoint": srv.URL + "/oauth2/device",
			"token_endpoint":                srv.URL + "/oauth2/token",
		})
	})
	mux.HandleFunc("/oauth2/device", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("client_id") != GrokClientID {
			t.Errorf("client_id %s", r.Form.Get("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"device_code":"dev-code",
			"user_code":"WDJB-MJHT",
			"verification_uri":"https://accounts.x.ai/oauth2/device",
			"expires_in":600,
			"interval":5
		}`)
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("device_code") != "dev-code" {
			t.Errorf("device_code %s", r.Form.Get("device_code"))
		}
		w.Header().Set("Content-Type", "application/json")
		n := tokenHits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`)
	})

	old := grokDiscoveryURL
	grokDiscoveryURL = srv.URL + "/.well-known/openid-configuration"
	t.Cleanup(func() { grokDiscoveryURL = old })

	d, err := StartGrokDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.UserCode != "WDJB-MJHT" {
		t.Fatalf("user_code = %s", d.UserCode)
	}
	if d.DeviceCode != "dev-code" {
		t.Fatalf("device_code = %s", d.DeviceCode)
	}
	if d.TokenURL != srv.URL+"/oauth2/token" {
		t.Fatalf("token_url = %s", d.TokenURL)
	}

	d.Interval = 1
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cred, err := d.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "at" || cred.RefreshToken != "rt" {
		t.Fatalf("%+v", cred)
	}
	if cred.Provider != ProviderGrok || cred.Source != "device_code" {
		t.Fatalf("%+v", cred)
	}
	if tokenHits.Load() < 2 {
		t.Fatalf("token hits %d, want pending then success", tokenHits.Load())
	}
}
