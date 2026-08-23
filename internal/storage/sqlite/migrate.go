package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{{
	version: 1,
	name:    "initial_schema",
	sql: `
CREATE TABLE tenants (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  timezone TEXT NOT NULL,
  active INTEGER NOT NULL CHECK (active IN (0,1)),
  created_at TEXT NOT NULL
);
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  email TEXT NOT NULL,
  display_name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  education_stage TEXT NOT NULL,
  birth_date TEXT,
  active INTEGER NOT NULL CHECK (active IN (0,1)),
  version INTEGER NOT NULL CHECK (version > 0),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tenant_id, email)
);
CREATE INDEX users_tenant_role_idx ON users(tenant_id, role, active);
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  user_id TEXT NOT NULL REFERENCES users(id),
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX sessions_user_active_idx ON sessions(tenant_id, user_id, revoked_at, expires_at);
CREATE TABLE curricula (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  code TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  stage TEXT NOT NULL,
  status TEXT NOT NULL,
  version INTEGER NOT NULL,
  revision INTEGER NOT NULL,
  created_by TEXT NOT NULL REFERENCES users(id),
  reviewed_by TEXT REFERENCES users(id),
  review_comment TEXT NOT NULL DEFAULT '',
  minimum_ethics_hours INTEGER NOT NULL,
  minimum_practice_mins INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tenant_id, code, revision)
);
CREATE INDEX curricula_filter_idx ON curricula(tenant_id, stage, status, updated_at);
CREATE TABLE curriculum_outcomes (
  curriculum_id TEXT NOT NULL REFERENCES curricula(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  kind TEXT NOT NULL,
  required INTEGER NOT NULL CHECK (required IN (0,1)),
  PRIMARY KEY(curriculum_id, code)
);
CREATE TABLE curriculum_prerequisites (
  curriculum_id TEXT NOT NULL REFERENCES curricula(id) ON DELETE CASCADE,
  prerequisite_id TEXT NOT NULL REFERENCES curricula(id),
  PRIMARY KEY(curriculum_id, prerequisite_id),
  CHECK(curriculum_id <> prerequisite_id)
);
CREATE TABLE offerings (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  curriculum_id TEXT NOT NULL REFERENCES curricula(id),
  curriculum_version INTEGER NOT NULL,
  code TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  starts_at TEXT NOT NULL,
  ends_at TEXT NOT NULL,
  capacity INTEGER NOT NULL CHECK (capacity > 0),
  confirmed_seats INTEGER NOT NULL CHECK (confirmed_seats >= 0),
  waitlisted_seats INTEGER NOT NULL CHECK (waitlisted_seats >= 0),
  educator_id TEXT NOT NULL REFERENCES users(id),
  minimum_age INTEGER NOT NULL,
  requires_guardian INTEGER NOT NULL CHECK (requires_guardian IN (0,1)),
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tenant_id, code)
);
CREATE INDEX offerings_window_idx ON offerings(tenant_id, status, starts_at, ends_at);
CREATE TABLE guardian_consents (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  guardian_name TEXT NOT NULL,
  scope TEXT NOT NULL,
  granted_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  version INTEGER NOT NULL
);
CREATE TABLE enrollments (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  offering_id TEXT NOT NULL REFERENCES offerings(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  status TEXT NOT NULL,
  guardian_consent_id TEXT REFERENCES guardian_consents(id),
  idempotency_key TEXT NOT NULL,
  completed_at TEXT,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tenant_id, offering_id, learner_id),
  UNIQUE(tenant_id, idempotency_key)
);
CREATE INDEX enrollments_status_idx ON enrollments(tenant_id, offering_id, status);
CREATE TABLE tool_policies (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  tool_code TEXT NOT NULL,
  display_name TEXT NOT NULL,
  allowed_data_classes TEXT NOT NULL,
  minimum_age INTEGER NOT NULL,
  requires_guardian INTEGER NOT NULL,
  requires_educator INTEGER NOT NULL,
  version INTEGER NOT NULL,
  active INTEGER NOT NULL,
  UNIQUE(tenant_id, tool_code)
);
CREATE TABLE data_consents (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  purpose TEXT NOT NULL,
  data_classes TEXT NOT NULL,
  granted_by TEXT NOT NULL REFERENCES users(id),
  granted_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  version INTEGER NOT NULL
);
CREATE INDEX data_consents_active_idx ON data_consents(tenant_id, learner_id, purpose, expires_at);
CREATE TABLE tool_grants (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  tool_policy_id TEXT NOT NULL REFERENCES tool_policies(id),
  consent_id TEXT NOT NULL REFERENCES data_consents(id),
  purpose TEXT NOT NULL,
  status TEXT NOT NULL,
  issued_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  version INTEGER NOT NULL
);
CREATE INDEX tool_grants_active_idx ON tool_grants(tenant_id, learner_id, status, expires_at);
CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  offering_id TEXT NOT NULL REFERENCES offerings(id),
  title TEXT NOT NULL,
  problem_statement TEXT NOT NULL,
  status TEXT NOT NULL,
  mentor_id TEXT REFERENCES users(id),
  approved_by TEXT REFERENCES users(id),
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE project_members (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  learner_id TEXT NOT NULL REFERENCES users(id),
  PRIMARY KEY(project_id, learner_id)
);
CREATE TABLE project_tool_grants (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  grant_id TEXT NOT NULL REFERENCES tool_grants(id),
  PRIMARY KEY(project_id, grant_id)
);
CREATE TABLE mentor_loads (
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  mentor_id TEXT NOT NULL REFERENCES users(id),
  active_projects INTEGER NOT NULL CHECK(active_projects >= 0),
  version INTEGER NOT NULL,
  PRIMARY KEY(tenant_id, mentor_id)
);
CREATE TABLE lab_resources (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  capacity INTEGER NOT NULL CHECK(capacity > 0),
  active INTEGER NOT NULL
);
CREATE TABLE lab_allocations (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  project_id TEXT NOT NULL REFERENCES projects(id),
  resource_id TEXT NOT NULL REFERENCES lab_resources(id),
  owner_id TEXT NOT NULL REFERENCES users(id),
  starts_at TEXT NOT NULL,
  ends_at TEXT NOT NULL,
  units INTEGER NOT NULL CHECK(units > 0),
  status TEXT NOT NULL,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  released_at TEXT
);
CREATE INDEX lab_allocations_conflict_idx ON lab_allocations(tenant_id, resource_id, status, starts_at, ends_at);
CREATE TABLE evidence_submissions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  enrollment_id TEXT NOT NULL REFERENCES enrollments(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  outcome_code TEXT NOT NULL,
  rubric_version INTEGER NOT NULL,
  attempt INTEGER NOT NULL,
  artifact_uri TEXT NOT NULL,
  artifact_sha256 TEXT NOT NULL,
  reflection TEXT NOT NULL,
  status TEXT NOT NULL,
  version INTEGER NOT NULL,
  submitted_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tenant_id, enrollment_id, outcome_code, attempt)
);
CREATE INDEX evidence_review_idx ON evidence_submissions(tenant_id, status, submitted_at);
CREATE TABLE review_decisions (
  id TEXT PRIMARY KEY,
  evidence_id TEXT NOT NULL REFERENCES evidence_submissions(id),
  reviewer_id TEXT NOT NULL REFERENCES users(id),
  outcome TEXT NOT NULL,
  score INTEGER NOT NULL,
  feedback TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE competencies (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  code TEXT NOT NULL,
  level INTEGER NOT NULL,
  evidence_id TEXT NOT NULL REFERENCES evidence_submissions(id),
  awarded_at TEXT NOT NULL,
  expires_at TEXT,
  version INTEGER NOT NULL,
  superseded INTEGER NOT NULL,
  superseded_at TEXT
);
CREATE INDEX competencies_active_idx ON competencies(tenant_id, learner_id, code, superseded, expires_at);
CREATE TABLE safety_incidents (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  learner_id TEXT NOT NULL REFERENCES users(id),
  tool_grant_id TEXT NOT NULL REFERENCES tool_grants(id),
  severity TEXT NOT NULL,
  summary TEXT NOT NULL,
  status TEXT NOT NULL,
  reported_by TEXT NOT NULL REFERENCES users(id),
  reported_at TEXT NOT NULL,
  resolved_at TEXT,
  version INTEGER NOT NULL
);
CREATE TABLE remediations (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  incident_id TEXT NOT NULL REFERENCES safety_incidents(id),
  assigned_to TEXT NOT NULL REFERENCES users(id),
  required_tasks TEXT NOT NULL,
  completed_tasks TEXT NOT NULL,
  due_at TEXT NOT NULL,
  version INTEGER NOT NULL
);
CREATE TABLE resource_packs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  title TEXT NOT NULL,
  license_code TEXT NOT NULL,
  content_uri TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  minimum_stage TEXT NOT NULL,
  maximum_stage TEXT NOT NULL,
  status TEXT NOT NULL,
  approved_by TEXT REFERENCES users(id),
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE resource_deliveries (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  pack_id TEXT NOT NULL REFERENCES resource_packs(id),
  offering_id TEXT NOT NULL REFERENCES offerings(id),
  recipient_id TEXT NOT NULL REFERENCES users(id),
  status TEXT NOT NULL,
  entitlement_key TEXT NOT NULL,
  lease_owner TEXT NOT NULL,
  lease_until TEXT,
  attempts INTEGER NOT NULL,
  last_error TEXT NOT NULL,
  delivered_at TEXT,
  acknowledged_at TEXT,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tenant_id, entitlement_key)
);
CREATE INDEX resource_deliveries_claim_idx ON resource_deliveries(status, lease_until, updated_at);
CREATE TABLE jobs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  kind TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  payload TEXT NOT NULL,
  status TEXT NOT NULL,
  available_at TEXT NOT NULL,
  lease_owner TEXT NOT NULL,
  lease_until TEXT,
  attempt INTEGER NOT NULL,
  max_attempts INTEGER NOT NULL,
  last_error TEXT NOT NULL,
  completed_at TEXT,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX jobs_claim_idx ON jobs(status, available_at, lease_until, kind);
CREATE TABLE audit_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  actor_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  action TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  result TEXT NOT NULL,
  metadata TEXT NOT NULL,
  occurred_at TEXT NOT NULL
);
CREATE INDEX audit_search_idx ON audit_events(tenant_id, occurred_at, action, object_type, object_id);
CREATE TABLE outbox_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TEXT NOT NULL,
  published_at TEXT,
  attempts INTEGER NOT NULL,
  last_error TEXT NOT NULL
);
CREATE INDEX outbox_pending_idx ON outbox_events(published_at, created_at);
CREATE TABLE idempotency_records (
  scope TEXT NOT NULL,
  key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  state TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  response TEXT NOT NULL,
  created_at TEXT NOT NULL,
  completed_at TEXT,
  PRIMARY KEY(scope, key)
);
`,
}}

func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
version INTEGER PRIMARY KEY,
name TEXT NOT NULL,
checksum TEXT NOT NULL,
applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("create schema migrations: %w", err)
	}
	for _, item := range migrations {
		if err := d.applyMigration(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) applyMigration(ctx context.Context, item migration) error {
	checksumBytes := sha256.Sum256([]byte(item.sql))
	checksum := hex.EncodeToString(checksumBytes[:])
	var existing string
	err := d.db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = ?", item.version).Scan(&existing)
	if err == nil {
		if existing != checksum {
			return fmt.Errorf("migration %d checksum changed", item.version)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read migration %d: %w", item.version, err)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.version, err)
	}
	if _, err := tx.ExecContext(ctx, item.sql); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %d: %w", item.version, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES(?,?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
		item.version, item.name, checksum,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", item.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.version, err)
	}
	return nil
}
