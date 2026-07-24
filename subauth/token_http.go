package subauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tokenResponse is the RFC 6749 subset we need.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

const defaultSkew = 30 * time.Second

func postFormToken(ctx context.Context, client *http.Client, tokenURL string, form url.Values) (tokenResponse, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, err
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return tokenResponse{}, fmt.Errorf("token parse (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || tr.AccessToken == "" {
		msg := strings.TrimSpace(tr.Error)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		// Never surface error_description (may echo secrets).
		return tokenResponse{}, fmt.Errorf("token endpoint: %s", msg)
	}
	return tr, nil
}

func expiryFrom(tr tokenResponse) time.Time {
	if tr.ExpiresIn <= 0 {
		return time.Time{}
	}
	d := time.Duration(tr.ExpiresIn) * time.Second
	if d > defaultSkew {
		d -= defaultSkew
	}
	return time.Now().Add(d)
}

// RefreshAccessToken exchanges a refresh token for a new access token.
func RefreshAccessToken(ctx context.Context, client *http.Client, tokenURL, clientID, refreshToken string) (Credential, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	tr, err := postFormToken(ctx, client, tokenURL, form)
	if err != nil {
		return Credential{}, err
	}
	c := Credential{
		AccessToken:  tr.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    tr.TokenType,
		ClientID:     clientID,
		TokenURL:     tokenURL,
		Expiry:       expiryFrom(tr),
		AccountID:    AccountIDFromTokens(tr.AccessToken, tr.IDToken),
	}
	if tr.RefreshToken != "" {
		c.RefreshToken = tr.RefreshToken
	}
	return c, nil
}
