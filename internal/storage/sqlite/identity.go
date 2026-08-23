package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
)

func (q *queries) InsertUser(ctx context.Context, user identity.User) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO users(
id, tenant_id, email, display_name, password_hash, role, education_stage, birth_date,
active, version, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		user.ID, user.TenantID, strings.ToLower(user.Email), user.DisplayName, user.PasswordHash,
		user.Role, user.EducationStage, nullableTime(user.BirthDate), user.Active, user.Version,
		user.CreatedAt.Format(time.RFC3339Nano), user.UpdatedAt.Format(time.RFC3339Nano),
	)
	return translateError("insert user", err)
}

func (q *queries) UpdateUser(ctx context.Context, user identity.User, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE users SET email=?, display_name=?, password_hash=?,
role=?, education_stage=?, birth_date=?, active=?, version=?, updated_at=?
WHERE tenant_id=? AND id=? AND version=?`,
		strings.ToLower(user.Email), user.DisplayName, user.PasswordHash, user.Role, user.EducationStage,
		nullableTime(user.BirthDate), user.Active, user.Version, user.UpdatedAt.Format(time.RFC3339Nano),
		user.TenantID, user.ID, expectedVersion,
	)
	return expectOne("update user", result, err)
}

func scanUser(scanner interface{ Scan(...any) error }) (identity.User, error) {
	var value identity.User
	var birthDate, createdAt, updatedAt sql.NullString
	err := scanner.Scan(
		&value.ID, &value.TenantID, &value.Email, &value.DisplayName, &value.PasswordHash,
		&value.Role, &value.EducationStage, &birthDate, &value.Active, &value.Version, &createdAt, &updatedAt,
	)
	if err != nil {
		return identity.User{}, err
	}
	var parseErr error
	value.BirthDate, parseErr = parseNullableTime(birthDate)
	if parseErr != nil {
		return identity.User{}, parseErr
	}
	if value.CreatedAt, parseErr = parseTime(createdAt.String); parseErr != nil {
		return identity.User{}, parseErr
	}
	if value.UpdatedAt, parseErr = parseTime(updatedAt.String); parseErr != nil {
		return identity.User{}, parseErr
	}
	return value, nil
}

const selectUser = `SELECT id, tenant_id, email, display_name, password_hash, role,
education_stage, birth_date, active, version, created_at, updated_at FROM users`

func (q *queries) GetUser(ctx context.Context, tenantID, userID common.ID) (identity.User, error) {
	value, err := scanUser(q.runner.QueryRowContext(ctx, selectUser+" WHERE tenant_id=? AND id=?", tenantID, userID))
	return value, translateError("get user", err)
}

func (q *queries) GetUserByEmail(ctx context.Context, tenantID common.ID, email string) (identity.User, error) {
	value, err := scanUser(q.runner.QueryRowContext(ctx, selectUser+" WHERE tenant_id=? AND email=?", tenantID, strings.ToLower(strings.TrimSpace(email))))
	return value, translateError("get user by email", err)
}

func (q *queries) InsertSession(ctx context.Context, session identity.Session) error {
	_, err := q.runner.ExecContext(ctx, `INSERT INTO sessions(
id, tenant_id, user_id, token_hash, expires_at, revoked_at, version, created_at)
VALUES(?,?,?,?,?,?,?,?)`, session.ID, session.TenantID, session.UserID, session.TokenHash,
		session.ExpiresAt.Format(time.RFC3339Nano), nullableTime(session.RevokedAt), session.Version,
		session.CreatedAt.Format(time.RFC3339Nano))
	return translateError("insert session", err)
}

func (q *queries) UpdateSession(ctx context.Context, session identity.Session, expectedVersion int64) error {
	result, err := q.runner.ExecContext(ctx, `UPDATE sessions SET expires_at=?, revoked_at=?, version=?
WHERE tenant_id=? AND id=? AND version=?`, session.ExpiresAt.Format(time.RFC3339Nano),
		nullableTime(session.RevokedAt), session.Version, session.TenantID, session.ID, expectedVersion)
	return expectOne("update session", result, err)
}

func (q *queries) RevokeSessionsForUser(ctx context.Context, tenantID, userID common.ID, now time.Time) (int64, error) {
	result, err := q.runner.ExecContext(ctx, `UPDATE sessions SET revoked_at=?, version=version+1
WHERE tenant_id=? AND user_id=? AND revoked_at IS NULL`, now.UTC().Format(time.RFC3339Nano), tenantID, userID)
	if err != nil {
		return 0, translateError("revoke user sessions", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("revoke user sessions rows: %w", err)
	}
	return count, nil
}

func (q *queries) GetSessionByTokenHash(ctx context.Context, tokenHash string) (identity.Session, error) {
	var value identity.Session
	var expiresAt, revokedAt, createdAt sql.NullString
	err := q.runner.QueryRowContext(ctx, `SELECT id, tenant_id, user_id, token_hash, expires_at,
revoked_at, version, created_at FROM sessions WHERE token_hash=?`, tokenHash).Scan(
		&value.ID, &value.TenantID, &value.UserID, &value.TokenHash, &expiresAt,
		&revokedAt, &value.Version, &createdAt,
	)
	if err != nil {
		return identity.Session{}, translateError("get session", err)
	}
	var parseErr error
	if value.ExpiresAt, parseErr = parseTime(expiresAt.String); parseErr != nil {
		return identity.Session{}, parseErr
	}
	if value.RevokedAt, parseErr = parseNullableTime(revokedAt); parseErr != nil {
		return identity.Session{}, parseErr
	}
	if value.CreatedAt, parseErr = parseTime(createdAt.String); parseErr != nil {
		return identity.Session{}, parseErr
	}
	return value, nil
}
