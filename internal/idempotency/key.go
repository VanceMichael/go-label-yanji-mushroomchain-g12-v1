package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

var ErrInvalidKey = errors.New("invalid idempotency key")

type Scope struct {
	TenantID string
	Method   string
	Path     string
	Key      string
}

func (s Scope) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" || strings.TrimSpace(s.Method) == "" ||
		strings.TrimSpace(s.Path) == "" || strings.TrimSpace(s.Key) == "" {
		return ErrInvalidKey
	}
	if len(s.Key) > 128 {
		return ErrInvalidKey
	}
	return nil
}

func (s Scope) Digest() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	canonical := strings.Join([]string{
		strings.TrimSpace(s.TenantID),
		strings.ToUpper(strings.TrimSpace(s.Method)),
		strings.TrimSpace(s.Path),
		strings.TrimSpace(s.Key),
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:]), nil
}
