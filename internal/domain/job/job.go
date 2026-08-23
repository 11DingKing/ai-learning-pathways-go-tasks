package job

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusLeased    Status = "leased"
	StatusSucceeded Status = "succeeded"
	StatusRetry     Status = "retry"
	StatusDead      Status = "dead"
)

type Job struct {
	ID            common.ID
	TenantID      common.ID
	Kind          string
	AggregateType string
	AggregateID   common.ID
	Payload       json.RawMessage
	Status        Status
	AvailableAt   time.Time
	LeaseOwner    string
	LeaseUntil    *time.Time
	Attempt       int
	MaxAttempts   int
	LastError     string
	CompletedAt   *time.Time
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func New(id, tenantID common.ID, kind, aggregateType string, aggregateID common.ID, payload any, availableAt time.Time, maxAttempts int, now time.Time) (Job, error) {
	if !id.Valid() || !tenantID.Valid() || !aggregateID.Valid() {
		return Job{}, common.FieldError{Field: "job", Message: "ids are required"}
	}
	kind = strings.TrimSpace(kind)
	aggregateType = strings.TrimSpace(aggregateType)
	if kind == "" || aggregateType == "" {
		return Job{}, common.FieldError{Field: "job", Message: "kind and aggregate type are required"}
	}
	if maxAttempts < 1 || maxAttempts > 50 {
		return Job{}, common.FieldError{Field: "max_attempts", Message: "must be between 1 and 50"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Job{}, fmt.Errorf("encode job payload: %w", err)
	}
	now = now.UTC()
	return Job{
		ID: id, TenantID: tenantID, Kind: kind, AggregateType: aggregateType,
		AggregateID: aggregateID, Payload: encoded, Status: StatusPending,
		AvailableAt: availableAt.UTC(), MaxAttempts: maxAttempts, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (j Job) Claimable(now time.Time) bool {
	if j.Status == StatusSucceeded || j.Status == StatusDead {
		return false
	}
	if j.Status == StatusLeased && j.LeaseUntil != nil && now.UTC().Before(*j.LeaseUntil) {
		return false
	}
	return !now.UTC().Before(j.AvailableAt)
}

func (j *Job) Lease(owner string, now time.Time, duration time.Duration) error {
	owner = strings.TrimSpace(owner)
	if owner == "" || duration <= 0 {
		return common.FieldError{Field: "lease", Message: "owner and duration are required"}
	}
	if !j.Claimable(now) {
		return common.ErrConflict
	}
	until := now.UTC().Add(duration)
	j.Status = StatusLeased
	j.LeaseOwner = owner
	j.LeaseUntil = &until
	j.Attempt++
	j.Version++
	j.UpdatedAt = now.UTC()
	return nil
}

func (j *Job) Heartbeat(owner string, now time.Time, duration time.Duration) error {
	if j.Status != StatusLeased || j.LeaseOwner != owner || j.LeaseUntil == nil || !now.UTC().Before(*j.LeaseUntil) {
		return common.ErrLeaseLost
	}
	until := now.UTC().Add(duration)
	j.LeaseUntil = &until
	j.Version++
	j.UpdatedAt = now.UTC()
	return nil
}

func (j *Job) Succeed(owner string, now time.Time) error {
	if j.Status != StatusLeased || j.LeaseOwner != owner || j.LeaseUntil == nil || !now.UTC().Before(*j.LeaseUntil) {
		return common.ErrLeaseLost
	}
	value := now.UTC()
	j.Status = StatusSucceeded
	j.CompletedAt = &value
	j.LeaseOwner = ""
	j.LeaseUntil = nil
	j.LastError = ""
	j.Version++
	j.UpdatedAt = value
	return nil
}

func (j *Job) Fail(owner string, cause error, now time.Time, backoff time.Duration) error {
	if j.Status != StatusLeased || j.LeaseOwner != owner {
		return common.ErrLeaseLost
	}
	if cause == nil {
		return common.FieldError{Field: "cause", Message: "is required"}
	}
	j.LastError = cause.Error()
	j.LeaseOwner = ""
	j.LeaseUntil = nil
	j.UpdatedAt = now.UTC()
	j.Version++
	if j.Attempt >= j.MaxAttempts {
		j.Status = StatusDead
		return nil
	}
	j.Status = StatusRetry
	j.AvailableAt = now.UTC().Add(backoff)
	return nil
}
