.PHONY: run deps tidy test fmt vet lint cover build docker compose-up compose-down migrate-up migrate-down benchmark

MIGRATE=go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

# Load .env if present so `make run` matches docker-compose credentials.
ifneq (,$(wildcard .env))
include .env
export
endif

run:
	go run ./cmd/api

deps:
	go mod download

tidy:
	go mod tidy

test:
	go test ./...

cover:
	go test ./... -cover

fmt:
	gofmt -w .

vet:
	go vet ./...

compose-up:
	docker compose up -d

compose-down:
	docker compose down

migrate-up:
	$(MIGRATE) -path ./migrations -database "$(DATABASE_URL)" up

migrate-down:
	$(MIGRATE) -path ./migrations -database "$(DATABASE_URL)" down 1

# Build a stripped, static binary into ./bin/api
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o bin/api ./cmd/api

# Build the Docker image locally (tag: urlshortener:dev)
docker:
	docker build -t urlshortener:dev .

# Run tests with race detector and show per-package coverage
test-race:
	go test -race -cover ./...

# Run golangci-lint (must be installed: go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.1)
lint:
	golangci-lint run --timeout 5m

# Redirect latency benchmark (requires: hey, jq, running stack via make compose-up)
# Results are printed to stdout and saved to docs/benchmark_raw.md.
benchmark:
	./scripts/benchmark.sh
