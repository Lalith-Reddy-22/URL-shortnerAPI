.PHONY: run deps tidy test fmt vet compose-up compose-down migrate-up migrate-down

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
