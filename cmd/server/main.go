package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/config"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/httpapi"
	"github.com/11DingKing/ai-learning-pathways-go/internal/observability"
	"github.com/11DingKing/ai-learning-pathways-go/internal/service"
	"github.com/11DingKing/ai-learning-pathways-go/internal/storage/sqlite"
	"github.com/11DingKing/ai-learning-pathways-go/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := observability.New(os.Getenv("APP_LOG_LEVEL"), os.Stdout)
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if cfg.BootstrapAdminPassword != "" {
		hash, err := service.HashPassword(cfg.BootstrapAdminPassword)
		if err != nil {
			return err
		}
		tenantID := common.ID(cfg.BootstrapTenantID)
		adminID, err := common.NewID("user")
		if err != nil {
			return err
		}
		admin, err := identity.NewUser(adminID, tenantID, cfg.BootstrapAdminEmail, "Program Administrator", hash, identity.RoleProgramAdmin, identity.StageUniversity, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := store.BootstrapIdentity(ctx, tenantID, cfg.BootstrapTenantName, "Asia/Shanghai", admin); err != nil {
			return err
		}
	}
	serviceLayer, err := service.New(store, common.SystemClock{}, cfg.SessionTTL)
	if err != nil {
		return err
	}
	server, err := httpapi.New(serviceLayer, store, logger, cfg.RequestTimeout)
	if err != nil {
		return err
	}
	handlers := map[string]worker.Handler{
		"evidence.review_requested": func(ctx context.Context, item job.Job) error {
			logger.Info("review job observed", "job_id", item.ID)
			return nil
		},
		"resource.deliver": func(ctx context.Context, item job.Job) error {
			logger.Info("delivery job observed", "job_id", item.ID)
			return nil
		},
	}
	background, stopBackground := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopBackground()
	workerService, err := worker.New(store, common.SystemClock{}, logger, cfg.WorkerID, cfg.WorkerPoll, cfg.WorkerLease, 20, handlers)
	if err != nil {
		return err
	}
	workerErr := make(chan error, 1)
	go func() { workerErr <- workerService.Run(background) }()
	httpServer := &http.Server{Addr: cfg.Address, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.RequestTimeout, WriteTimeout: cfg.RequestTimeout, IdleTimeout: 60 * time.Second}
	serveErr := make(chan error, 1)
	go func() { logger.Info("server started", "address", cfg.Address); serveErr <- httpServer.ListenAndServe() }()
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case err := <-workerErr:
		if !errors.Is(err, context.Canceled) {
			return err
		}
	case <-background.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	stopBackground()
	select {
	case err := <-workerErr:
		if !errors.Is(err, context.Canceled) {
			return err
		}
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
	return nil
}
