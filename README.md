# AI Learning Pathways

AI Learning Pathways is a Go backend for governing artificial-intelligence education across primary, secondary, university, vocational, and lifelong-learning programs.

The service models curriculum publication, prerequisite graphs, offerings and capacity-safe enrollment, project mentoring, safe-tool consent, evidence review, competency progression, resource delivery, audit events, idempotency, and durable worker jobs in SQLite.

## Run

Set `APP_BOOTSTRAP_ADMIN_PASSWORD` and start with `go run ./cmd/server`. The default HTTP address is `:8080`; readiness is available at `/readyz`.

## Verify

`make test`, `make race`, `make vet`, and `make build` run the local quality gates. `make docker` builds the production image.
