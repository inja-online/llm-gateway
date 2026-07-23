package subauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PkceCodes holds a PKCE S256 verifier/challenge pair.
type PkceCodes struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a new S256 PKCE pair (URL-safe base64, no padding).
func GeneratePKCE() (PkceCodes, error) {
	var b [64]byte
	if _, err := rand.Read(b[:]); err != nil {
		return PkceCodes{}, fmt.Errorf("pkce: rand: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return PkceCodes{Verifier: verifier, Challenge: challenge}, nil
}

// RandomState returns a random URL-safe state string.
func RandomState() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
