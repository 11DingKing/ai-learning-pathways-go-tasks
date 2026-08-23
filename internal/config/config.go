package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address                string
	DatabasePath           string
	ShutdownTimeout        time.Duration
	RequestTimeout         time.Duration
	SessionTTL             time.Duration
	WorkerID               string
	WorkerPoll             time.Duration
	WorkerLease            time.Duration
	BootstrapTenantID      string
	BootstrapTenantName    string
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

func Load() (Config, error) {
	result := Config{
		Address: env("APP_ADDRESS", ":8080"), DatabasePath: env("APP_DATABASE_PATH", "./data/pathways.db"),
		ShutdownTimeout: duration("APP_SHUTDOWN_TIMEOUT", 15*time.Second), RequestTimeout: duration("APP_REQUEST_TIMEOUT", 20*time.Second),
		SessionTTL: duration("APP_SESSION_TTL", 12*time.Hour), WorkerID: env("APP_WORKER_ID", hostname()),
		WorkerPoll: duration("APP_WORKER_POLL", 500*time.Millisecond), WorkerLease: duration("APP_WORKER_LEASE", 30*time.Second),
		BootstrapTenantID: env("APP_BOOTSTRAP_TENANT_ID", "tenant_demo"), BootstrapTenantName: env("APP_BOOTSTRAP_TENANT_NAME", "AI Learning Pathways"),
		BootstrapAdminEmail: env("APP_BOOTSTRAP_ADMIN_EMAIL", "admin@example.edu"), BootstrapAdminPassword: os.Getenv("APP_BOOTSTRAP_ADMIN_PASSWORD"),
	}
	if !strings.Contains(result.Address, ":") {
		return Config{}, fmt.Errorf("APP_ADDRESS must include a port")
	}
	if strings.TrimSpace(result.DatabasePath) == "" {
		return Config{}, fmt.Errorf("APP_DATABASE_PATH is required")
	}
	if result.RequestTimeout <= 0 || result.ShutdownTimeout <= 0 || result.WorkerPoll <= 0 || result.WorkerLease <= result.WorkerPoll {
		return Config{}, fmt.Errorf("timeouts and worker lease are invalid")
	}
	return result, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func duration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
func hostname() string {
	value, err := os.Hostname()
	if err != nil || value == "" {
		return "pathways-worker"
	}
	return value + "-" + strconv.Itoa(os.Getpid())
}
