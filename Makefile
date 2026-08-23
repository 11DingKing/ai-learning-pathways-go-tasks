.PHONY: test race vet build run docker

test:
	GOTOOLCHAIN=local go test ./... -count=1

race:
	GOTOOLCHAIN=local go test -race ./... -count=1

vet:
	GOTOOLCHAIN=local go vet ./...

build:
	GOTOOLCHAIN=local go build ./...

run:
	APP_BOOTSTRAP_ADMIN_PASSWORD=change-this-password APP_DATABASE_PATH=./data/pathways.db go run ./cmd/server

docker:
	docker build -t ai-learning-pathways-go:base .
