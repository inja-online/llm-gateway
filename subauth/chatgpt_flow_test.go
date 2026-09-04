package subauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStartChatGPTFlow(t *testing.T) {
	f, err := StartChatGPTFlow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if !strings.Contains(f.AuthorizeURL, "auth.openai.com") {
		t.Fatalf("url = %s", f.AuthorizeURL)
	}
	u, err := url.Parse(f.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge") == "" {
		t.Fatalf("missing code_challenge: %s", f.AuthorizeURL)
	}
	redir := q.Get("redirect_uri")
	if !strings.HasPrefix(redir, "http://localhost:") || !strings.HasSuffix(redir, "/auth/callback") {
		t.Fatalf("redirect_uri = %s", redir)
	}
	if q.Get("state") == "" {
		t.Fatal("missing state")
	}

	f.Close()
	f.Close()
}

func TestChatGPTFlowWaitExchangesCode(t *testing.T) {
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "test-code" {
			t.Errorf("form %v", r.Form)
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("missing code_verifier")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tok.Close()

	f, err := StartChatGPTFlow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.tokenURL = tok.URL

	u, err := url.Parse(f.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	redir := u.Query().Get("redirect_uri")
	state := u.Query().Get("state")
	cbURL := redir + "?code=test-code&state=" + url.QueryEscape(state)

	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = client.Get(cbURL)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("callback %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cred, err := f.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cred.AccessToken != "at" || cred.RefreshToken != "rt" {
		t.Fatalf("%+v", cred)
	}
	if cred.Provider != ProviderChatGPT || cred.Source != "oauth_pkce" {
		t.Fatalf("%+v", cred)
	}
	if cred.ClientID != ChatGPTClientID {
		t.Fatalf("client_id %s", cred.ClientID)
	}
}
