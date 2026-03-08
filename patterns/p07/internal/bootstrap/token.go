package bootstrap

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// tokenClaims is the payload embedded in a vended token.
type tokenClaims struct {
	DeviceCN  string `json:"sub"` // device Common Name from client cert
	ExpiresAt int64  `json:"exp"` // Unix seconds
}

// Tokener issues and validates short-lived HMAC-SHA256-signed tokens.
// In production these tokens would be NATS user JWTs or AWS STS credentials;
// the HMAC token here carries the same semantics without requiring broker-specific
// infrastructure in the pattern demo.
type Tokener struct {
	secret []byte
	ttl    time.Duration
}

// NewTokener creates a Tokener with the given HMAC secret and token lifetime.
func NewTokener(secret string, ttl time.Duration) *Tokener {
	return &Tokener{secret: []byte(secret), ttl: ttl}
}

// Issue generates a signed token for the given device CN.
// Returns the token string and the time at which it expires.
func (t *Tokener) Issue(deviceCN string) (token string, expiresAt time.Time, err error) {
	expiresAt = time.Now().Add(t.ttl)
	claims := tokenClaims{DeviceCN: deviceCN, ExpiresAt: expiresAt.Unix()}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal claims: %w", err)
	}

	enc := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(enc, t.secret)
	return enc + "." + sig, expiresAt, nil
}

// Validate checks the token signature and expiry, returning the device CN on success.
func (t *Tokener) Validate(token string) (deviceCN string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed token")
	}
	enc, sig := parts[0], parts[1]

	if !hmac.Equal([]byte(sign(enc, t.secret)), []byte(sig)) {
		return "", errors.New("invalid token signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}

	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("unmarshal token: %w", err)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return "", errors.New("token expired")
	}

	return claims.DeviceCN, nil
}

func sign(data string, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
