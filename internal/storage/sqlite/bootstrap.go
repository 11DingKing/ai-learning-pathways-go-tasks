package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

func (d *DB) BootstrapIdentity(ctx context.Context, tenantID common.ID, tenantName, timezone string, admin identity.User) error {
	if !tenantID.Valid() || tenantName == "" || timezone == "" {
		return common.FieldError{Field: "bootstrap", Message: "tenant id, name, and timezone are required"}
	}
	return d.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		queries, ok := tx.(*queries)
		if !ok {
			return fmt.Errorf("unexpected transaction implementation")
		}
		_, err := queries.runner.ExecContext(ctx, `INSERT INTO tenants(id, name, timezone, active, created_at)
VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, tenantID, tenantName, timezone, true, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return translateError("bootstrap tenant", err)
		}
		if !admin.ID.Valid() {
			return nil
		}
		var existing int
		if err := queries.runner.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id=? AND email=?`, tenantID, admin.Email).Scan(&existing); err != nil {
			return translateError("check bootstrap admin", err)
		}
		if existing > 0 {
			return nil
		}
		return tx.InsertUser(ctx, admin)
	})
}
