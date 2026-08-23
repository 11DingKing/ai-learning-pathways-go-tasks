package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type Service struct {
	store      repository.Store
	clock      common.Clock
	sessionTTL time.Duration
}

func New(store repository.Store, clock common.Clock, sessionTTL time.Duration) (*Service, error) {
	if store == nil || clock == nil {
		return nil, common.FieldError{Field: "service", Message: "store and clock are required"}
	}
	if sessionTTL < 5*time.Minute || sessionTTL > 30*24*time.Hour {
		return nil, common.FieldError{Field: "session_ttl", Message: "must be between five minutes and thirty days"}
	}
	return &Service{store: store, clock: clock, sessionTTL: sessionTTL}, nil
}

type Actor struct {
	TenantID  common.ID
	UserID    common.ID
	Role      identity.Role
	RequestID string
}

func (a Actor) Valid() bool { return a.TenantID.Valid() && a.UserID.Valid() && a.Role != "" }

func (s *Service) authorizedUser(ctx context.Context, tx repository.Reader, actor Actor, action string) (identity.User, error) {
	if !actor.Valid() {
		return identity.User{}, common.ErrUnauthenticated
	}
	user, err := tx.GetUser(ctx, actor.TenantID, actor.UserID)
	if err != nil {
		return identity.User{}, fmt.Errorf("load actor: %w", err)
	}
	if user.Role != actor.Role {
		return identity.User{}, fmt.Errorf("actor role changed: %w", common.ErrForbidden)
	}
	if err := user.Require(action); err != nil {
		return identity.User{}, err
	}
	return user, nil
}

func randomSecret(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func newID(prefix string) (common.ID, error) { return common.NewID(prefix) }

func audit(ctx context.Context, tx repository.Tx, actor Actor, action, objectType string, objectID common.ID, result string, metadata any, now time.Time) error {
	id, err := newID("audit")
	if err != nil {
		return err
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	return tx.InsertAudit(ctx, repository.AuditEvent{ID: id, TenantID: actor.TenantID, ActorID: actor.UserID,
		RequestID: actor.RequestID, Action: action, ObjectType: objectType, ObjectID: objectID,
		Result: result, Metadata: body, OccurredAt: now})
}

func outbox(ctx context.Context, tx repository.Tx, tenantID common.ID, aggregateType string, aggregateID common.ID, kind string, payload any, now time.Time) error {
	id, err := newID("event")
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode outbox payload: %w", err)
	}
	return tx.InsertOutbox(ctx, repository.OutboxEvent{ID: id, TenantID: tenantID,
		AggregateType: aggregateType, AggregateID: aggregateID, Kind: kind, Payload: body, CreatedAt: now})
}
