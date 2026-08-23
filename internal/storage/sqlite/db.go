package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, common.FieldError{Field: "database_path", Message: "is required"}
	}
	dsn := path
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(8)
	database.SetMaxIdleConns(4)
	database.SetConnMaxIdleTime(5 * time.Minute)
	if path == ":memory:" {
		// SQLite's private in-memory database is scoped to one physical connection.
		database.SetMaxOpenConns(1)
		database.SetMaxIdleConns(1)
	}
	result := &DB{db: database}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := result.Ping(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := result.Migrate(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return result, nil
}

func (d *DB) Ping(ctx context.Context) error {
	if err := d.db.PingContext(ctx); err != nil {
		return common.WrapDependency("ping sqlite", err)
	}
	return nil
}

func (d *DB) Close() error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("close sqlite: %w", err)
	}
	return nil
}

func (d *DB) WithinTx(ctx context.Context, fn func(context.Context, repository.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return common.WrapDependency("begin transaction", err)
	}
	wrapped := &queries{runner: tx}
	if err := fn(ctx, wrapped); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return fmt.Errorf("business operation failed: %w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("commit cancelled transaction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return common.WrapDependency("commit transaction", err)
	}
	return nil
}

func (d *DB) Read(ctx context.Context, fn func(context.Context, repository.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read cancelled: %w", err)
	}
	return fn(ctx, &queries{runner: d.db})
}

type sqlRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type queries struct {
	runner sqlRunner
}

var _ repository.Tx = (*queries)(nil)

func translateError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, common.ErrNotFound)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
		return fmt.Errorf("%s: %w: %v", operation, common.ErrConflict, err)
	}
	if strings.Contains(message, "database is locked") || strings.Contains(message, "busy") {
		return common.WrapDependency(operation, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func expectOne(operation string, result sql.Result, err error) error {
	if err != nil {
		return translateError(operation, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", operation, err)
	}
	if count != 1 {
		return fmt.Errorf("%s affected %d rows: %w", operation, count, common.ErrConflict)
	}
	return nil
}

func nullableID(value *common.ID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	result, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse persisted time %q: %w", value, err)
	}
	return result.UTC(), nil
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	result, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
