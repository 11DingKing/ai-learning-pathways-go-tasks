package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
)

type Handler func(context.Context, job.Job) error

type Worker struct {
	store                    repository.Store
	clock                    common.Clock
	logger                   *slog.Logger
	owner                    string
	poll, lease, taskTimeout time.Duration
	batch                    int
	handlers                 map[string]Handler
}

func New(store repository.Store, clock common.Clock, logger *slog.Logger, owner string, poll, lease time.Duration, batch int, handlers map[string]Handler) (*Worker, error) {
	if store == nil || clock == nil || logger == nil || owner == "" {
		return nil, common.FieldError{Field: "worker", Message: "dependencies and owner are required"}
	}
	if poll <= 0 || lease <= 2*poll || batch < 1 || batch > 100 {
		return nil, common.FieldError{Field: "worker", Message: "poll, lease, or batch is invalid"}
	}
	copyHandlers := make(map[string]Handler, len(handlers))
	for kind, handler := range handlers {
		if kind == "" || handler == nil {
			return nil, common.FieldError{Field: "handler", Message: "kind and handler are required"}
		}
		copyHandlers[kind] = handler
	}
	return &Worker{store: store, clock: clock, logger: logger, owner: owner, poll: poll, lease: lease, taskTimeout: lease * 3, batch: batch, handlers: copyHandlers}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		if err := w.cycle(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("worker cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) cycle(ctx context.Context) error {
	kinds := make([]string, 0, len(w.handlers))
	for kind := range w.handlers {
		kinds = append(kinds, kind)
	}
	var claimed []job.Job
	err := w.store.WithinTx(ctx, func(ctx context.Context, tx repository.Tx) error {
		var err error
		claimed, err = tx.ClaimJobs(ctx, w.owner, w.clock.Now(), w.lease, kinds, w.batch)
		return err
	})
	if err != nil {
		return fmt.Errorf("claim jobs: %w", err)
	}
	var group sync.WaitGroup
	for _, item := range claimed {
		item := item
		group.Add(1)
		go func() { defer group.Done(); w.process(ctx, item) }()
	}
	group.Wait()
	return nil
}

func (w *Worker) process(parent context.Context, claimed job.Job) {
	handler := w.handlers[claimed.Kind]
	base := context.WithoutCancel(parent)
	ctx, cancel := context.WithTimeout(base, w.taskTimeout)
	defer cancel()
	stopHeartbeat := make(chan struct{})
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() { defer heartbeat.Done(); w.heartbeat(ctx, claimed.TenantID, claimed.ID, stopHeartbeat) }()
	err := handler(ctx, claimed)
	close(stopHeartbeat)
	heartbeat.Wait()
	now := w.clock.Now()
	persistErr := w.store.WithinTx(base, func(ctx context.Context, tx repository.Tx) error {
		current, loadErr := tx.GetJob(ctx, claimed.TenantID, claimed.ID)
		if loadErr != nil {
			return loadErr
		}
		expected := current.Version
		if err == nil {
			if transitionErr := current.Succeed(w.owner, now); transitionErr != nil {
				return transitionErr
			}
		} else {
			if transitionErr := current.Fail(w.owner, err, now, retryBackoff(current.Attempt)); transitionErr != nil {
				return transitionErr
			}
		}
		return tx.UpdateJob(ctx, current, expected)
	})
	if persistErr != nil {
		w.logger.Error("persist job result", "job_id", claimed.ID, "error", persistErr)
		return
	}
	if err != nil {
		w.logger.Warn("job handler failed", "job_id", claimed.ID, "kind", claimed.Kind, "error", err)
		return
	}
	w.logger.Info("job completed", "job_id", claimed.ID, "kind", claimed.Kind)
}

func (w *Worker) heartbeat(ctx context.Context, tenantID, jobID common.ID, stop <-chan struct{}) {
	ticker := time.NewTicker(w.lease / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			_ = w.store.WithinTx(context.WithoutCancel(ctx), func(ctx context.Context, tx repository.Tx) error {
				current, err := tx.GetJob(ctx, tenantID, jobID)
				if err != nil {
					return err
				}
				expected := current.Version
				if err := current.Heartbeat(w.owner, w.clock.Now(), w.lease); err != nil {
					return err
				}
				return tx.UpdateJob(ctx, current, expected)
			})
		}
	}
}

func retryBackoff(attempt int) time.Duration {
	power := math.Pow(2, float64(max(0, attempt-1)))
	delay := time.Duration(power) * time.Second
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}
