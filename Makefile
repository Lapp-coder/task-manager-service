LINT_VERSION := v2.15.0
ENV_FILE ?= .env
COMPOSE := docker compose --env-file $(ENV_FILE)

.PHONY: build test test-integration lint up down logs migrate

build:
	go build -o bin/server ./cmd/server
	go build -o bin/migrate ./cmd/migrate

test:
	go test -race ./...

test-integration:
	go test -race -tags integration ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION) run ./...

$(ENV_FILE):
	@umask 077; sed \
		-e "s/^MYSQL_PASSWORD=$$/MYSQL_PASSWORD=$$(openssl rand -hex 16)/" \
		-e "s/^MYSQL_ROOT_PASSWORD=$$/MYSQL_ROOT_PASSWORD=$$(openssl rand -hex 16)/" \
		-e "s/^JWT_SECRET=$$/JWT_SECRET=$$(openssl rand -hex 32)/" \
		.env.example > $@
	@echo "created $@ with generated secrets"

up: $(ENV_FILE)
	$(COMPOSE) up --build -d --wait
	@echo "API: http://localhost:$$($(COMPOSE) port app 8080 | cut -d: -f2)/swagger/"

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f app

migrate:
	go run ./cmd/migrate up
