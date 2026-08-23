package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type LoginResult struct {
	Token   string
	Session identity.Session
	User    identity.User
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 200 {
		return "", common.FieldError{Field: "password", Message: "must contain 12 to 200 characters"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func (s *Service) Login(ctx context.Context, tenantID common.ID, email, password string) (LoginResult, error) {
	if !tenantID.Valid() || strings.TrimSpace(email) == "" || password == "" {
		return LoginResult{}, common.ErrUnauthenticated
	}
	now := s.clock.Now()
	token, err := randomSecret(32)
	if err != nil {
		return LoginResult{}, err
	}
	sessionID, err := newID("session")
	if err != nil {
		return LoginResult{}, err
	}
	var result LoginResult
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		user, err := tx.GetUserByEmail(ctx, tenantID, email)
		if err != nil {
			return common.ErrUnauthenticated
		}
		if !user.Active || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			return common.ErrUnauthenticated
		}
		session, err := identity.NewSession(sessionID, tenantID, user.ID, token, now, now.Add(s.sessionTTL))
		if err != nil {
			return err
		}
		if err := tx.InsertSession(ctx, session); err != nil {
			return fmt.Errorf("persist login session: %w", err)
		}
		actor := Actor{TenantID: tenantID, UserID: user.ID, Role: user.Role}
		if err := audit(ctx, tx, actor, "identity.login", "session", session.ID, "success", map[string]any{"expires_at": session.ExpiresAt}, now); err != nil {
			return err
		}
		result = LoginResult{Token: token, Session: session, User: user}
		return nil
	})
	return result, err
}

func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	if len(token) < 32 {
		return Actor{}, common.ErrUnauthenticated
	}
	now := s.clock.Now()
	var actor Actor
	err := s.store.Read(ctx, func(ctx context.Context, reader repository.Reader) error {
		session, err := reader.GetSessionByTokenHash(ctx, identity.HashToken(token))
		if err != nil {
			return common.AuthenticationLookupError(err)
		}
		if err := session.Validate(token, now); err != nil {
			return err
		}
		user, err := reader.GetUser(ctx, session.TenantID, session.UserID)
		if err != nil {
			return fmt.Errorf("load session user: %w", err)
		}
		if !user.Active {
			return common.ErrUnauthenticated
		}
		actor = Actor{TenantID: user.TenantID, UserID: user.ID, Role: user.Role}
		return nil
	})
	return actor, err
}

func (s *Service) Logout(ctx context.Context, actor Actor, token string) error {
	now := s.clock.Now()
	return s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		session, err := tx.GetSessionByTokenHash(ctx, identity.HashToken(token))
		if err != nil {
			return common.ErrUnauthenticated
		}
		if session.TenantID != actor.TenantID || session.UserID != actor.UserID {
			return common.ErrForbidden
		}
		expected := session.Version
		if err := session.Revoke(now); err != nil {
			return err
		}
		if err := tx.UpdateSession(ctx, session, expected); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		return audit(ctx, tx, actor, "identity.logout", "session", session.ID, "success", nil, now)
	})
}

func (s *Service) RegisterUser(ctx context.Context, actor Actor, email, displayName, password string, role identity.Role, stage identity.EducationStage, birthDate *time.Time) (identity.User, error) {
	now := s.clock.Now()
	hash, err := HashPassword(password)
	if err != nil {
		return identity.User{}, err
	}
	id, err := newID("user")
	if err != nil {
		return identity.User{}, err
	}
	user, err := identity.NewUser(id, actor.TenantID, email, displayName, hash, role, stage, now)
	if err != nil {
		return identity.User{}, err
	}
	user.BirthDate = birthDate
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		if _, err := s.authorizedUser(ctx, tx, actor, "program.admin"); err != nil {
			return err
		}
		if err := tx.InsertUser(ctx, user); err != nil {
			return fmt.Errorf("register user: %w", err)
		}
		return audit(ctx, tx, actor, "identity.user_registered", "user", user.ID, "success", map[string]any{"role": user.Role}, now)
	})
	return user, err
}
