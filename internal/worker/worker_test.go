package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
	"github.com/11DingKing/ai-learning-pathways-go/internal/storage/sqlite"
	"github.com/11DingKing/ai-learning-pathways-go/internal/worker"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestWorkerClaimsProcessesAndAcknowledgesJob(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 23, 11, 0, 0, 0, time.UTC)
	tenant := common.ID("tenant_worker")
	user, _ := identity.NewUser(common.ID("user_worker"), tenant, "worker@example.edu", "Worker", "hash", identity.RoleProgramAdmin, identity.StageUniversity, now)
	if err := db.BootstrapIdentity(context.Background(), tenant, "Worker Tenant", "UTC", user); err != nil {
		t.Fatal(err)
	}
	value, err := job.New(common.ID("job_worker"), tenant, "test.kind", "aggregate", common.ID("aggregate_worker"), map[string]string{"value": "ok"}, now, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertJob(ctx, value) }); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	handled := make(chan struct{}, 1)
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	service, err := worker.New(db, fixedClock{now}, logger, "worker-a", 10*time.Millisecond, 100*time.Millisecond, 5, map[string]worker.Handler{"test.kind": func(ctx context.Context, item job.Job) error { calls.Add(1); handled <- struct{}{}; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	select {
	case <-handled:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("worker did not process job")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("worker stopped with unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler called %d times", calls.Load())
	}
	if err := db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		loaded, err := reader.GetJob(ctx, tenant, value.ID)
		if err != nil {
			return err
		}
		if loaded.Status != job.StatusSucceeded || loaded.LeaseOwner != "" {
			t.Fatalf("job not succeeded: %+v", loaded)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Helper(); return len(p), nil }
