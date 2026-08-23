package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/config"
)

func withEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range values {
		old, exists := os.LookupEnv(key)
		if value == "" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, value)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func TestLoadDefaultsAreOperational(t *testing.T) {
	keys := []string{"APP_ADDRESS", "APP_DATABASE_PATH", "APP_SESSION_TTL", "APP_REQUEST_TIMEOUT", "APP_SHUTDOWN_TIMEOUT", "APP_WORKER_ID", "APP_WORKER_POLL", "APP_WORKER_LEASE", "APP_BOOTSTRAP_TENANT_ID", "APP_BOOTSTRAP_TENANT_NAME", "APP_BOOTSTRAP_ADMIN_EMAIL", "APP_BOOTSTRAP_ADMIN_PASSWORD"}
	for _, key := range keys {
		_ = os.Unsetenv(key)
	}
	value, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if value.Address != ":8080" || value.DatabasePath == "" || value.SessionTTL != 12*time.Hour {
		t.Fatalf("unexpected defaults: %+v", value)
	}
	if value.WorkerID == "" || value.WorkerPoll <= 0 || value.WorkerLease <= value.WorkerPoll {
		t.Fatalf("worker defaults invalid: %+v", value)
	}
}

func TestLoadReadsExplicitConfiguration(t *testing.T) {
	withEnv(t, map[string]string{"APP_ADDRESS": "127.0.0.1:9191", "APP_DATABASE_PATH": "/tmp/pathways-test.db", "APP_SESSION_TTL": "2h", "APP_REQUEST_TIMEOUT": "3s", "APP_SHUTDOWN_TIMEOUT": "4s", "APP_WORKER_ID": "worker-explicit", "APP_WORKER_POLL": "100ms", "APP_WORKER_LEASE": "2s", "APP_BOOTSTRAP_TENANT_ID": "tenant-explicit", "APP_BOOTSTRAP_TENANT_NAME": "Explicit Tenant", "APP_BOOTSTRAP_ADMIN_EMAIL": "owner@example.edu", "APP_BOOTSTRAP_ADMIN_PASSWORD": "secret-password"})
	value, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if value.Address != "127.0.0.1:9191" || value.DatabasePath != "/tmp/pathways-test.db" || value.WorkerID != "worker-explicit" {
		t.Fatalf("explicit values lost: %+v", value)
	}
	if value.SessionTTL != 2*time.Hour || value.RequestTimeout != 3*time.Second || value.ShutdownTimeout != 4*time.Second || value.WorkerLease != 2*time.Second {
		t.Fatalf("duration values lost: %+v", value)
	}
}

func TestLoadRejectsMalformedAddressAndEmptyDatabase(t *testing.T) {
	withEnv(t, map[string]string{"APP_ADDRESS": "8080"})
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "APP_ADDRESS") {
		t.Fatalf("expected address error, got %v", err)
	}
	withEnv(t, map[string]string{"APP_ADDRESS": ":8080", "APP_DATABASE_PATH": "   "})
	value, err := config.Load()
	if err != nil || value.DatabasePath == "" {
		t.Fatalf("blank database path should use default, got value=%+v err=%v", value, err)
	}
}

func TestInvalidDurationFallsBackWithoutBreakingStartup(t *testing.T) {
	withEnv(t, map[string]string{"APP_SESSION_TTL": "not-a-duration", "APP_REQUEST_TIMEOUT": "also-invalid", "APP_WORKER_POLL": "invalid", "APP_WORKER_LEASE": "invalid"})
	value, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if value.SessionTTL != 12*time.Hour || value.RequestTimeout != 20*time.Second || value.WorkerPoll != 500*time.Millisecond || value.WorkerLease != 30*time.Second {
		t.Fatalf("invalid duration fallback changed: %+v", value)
	}
}

func TestLoadRejectsUnsafeWorkerRelationship(t *testing.T) {
	withEnv(t, map[string]string{"APP_WORKER_POLL": "2s", "APP_WORKER_LEASE": "1s"})
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "worker lease") {
		t.Fatalf("expected worker relationship error, got %v", err)
	}
}
