package identity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type Session struct {
	ID        common.ID
	TenantID  common.ID
	UserID    common.ID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	Version   int64
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewSession(id, tenantID, userID common.ID, token string, now, expiresAt time.Time) (Session, error) {
	if !id.Valid() || !tenantID.Valid() || !userID.Valid() {
		return Session{}, common.FieldError{Field: "session", Message: "ids are required"}
	}
	if len(token) < 32 {
		return Session{}, common.FieldError{Field: "token", Message: "must have at least 32 characters"}
	}
	now = now.UTC()
	expiresAt = expiresAt.UTC()
	if !expiresAt.After(now) {
		return Session{}, common.FieldError{Field: "expires_at", Message: "must be in the future"}
	}
	return Session{
		ID: id, TenantID: tenantID, UserID: userID, TokenHash: HashToken(token),
		ExpiresAt: expiresAt, CreatedAt: now, Version: 1,
	}, nil
}

func (s Session) Validate(token string, now time.Time) error {
	provided, err := hex.DecodeString(HashToken(token))
	if err != nil {
		return fmt.Errorf("decode provided token: %w", err)
	}
	expected, err := hex.DecodeString(s.TokenHash)
	if err != nil {
		return fmt.Errorf("decode stored token: %w", err)
	}
	if len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
		return common.ErrUnauthenticated
	}
	if s.RevokedAt != nil {
		return fmt.Errorf("session revoked at %s: %w", s.RevokedAt.Format(time.RFC3339), common.ErrUnauthenticated)
	}
	if !now.UTC().Before(s.ExpiresAt) {
		return fmt.Errorf("session expired at %s: %w", s.ExpiresAt.Format(time.RFC3339), common.ErrExpired)
	}
	return nil
}

func (s *Session) Revoke(now time.Time) error {
	if s.RevokedAt != nil {
		return nil
	}
	value := now.UTC()
	s.RevokedAt = &value
	s.Version++
	return nil
}
