LINT_VERSION := v2.15.0

.PHONY: build test test-integration lint up down migrate

build:
	go build -o bin/server ./cmd/server
	go build -o bin/migrate ./cmd/migrate

test:
	go test -race ./...

test-integration:
	go test -race -tags integration ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION) run ./...

up:
	docker compose up --build -d

down:
	docker compose down

migrate:
	go run ./cmd/migrate up
