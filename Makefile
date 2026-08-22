# Mark8ly — top-level dev commands.
#
# Conventions:
#   - All commands are runnable from the repo root.
#   - SERVICE=<name> selects which Go service for migrate/seed targets.
#   - Local stack lives in infra/dev/docker-compose.yml.

COMPOSE := docker compose -f infra/dev/docker-compose.yml --project-directory infra/dev

.PHONY: help dev dev-secrets dev-down dev-logs dev-clean build test test-unit test-int cover lint check-types e2e \
        migrate-up migrate-down migrate-version migrate-new seed go-tidy go-build clean

help:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev-secrets: ## Pull GIP/OAuth secrets from GCP Secret Manager into infra/dev/.env.local
	@./infra/dev/load-secrets.sh

dev: dev-secrets ## Bring up the full local stack (auto-loads secrets first)
	$(COMPOSE) up --build

dev-down: ## Stop the local stack
	$(COMPOSE) down

dev-clean: ## Stop and remove volumes (wipes Postgres data)
	$(COMPOSE) down -v

dev-logs: ## Tail container logs
	$(COMPOSE) logs -f

build: ## Turborepo build of every workspace
	npx turbo run build

test: test-unit ## Run all unit tests (alias for test-unit)

test-unit: ## Run unit tests across every Go service and TS workspace
	@echo "▶ platform-api"
	@cd services/platform-api && go test ./...
	@echo "▶ auth-bff"
	@cd services/auth-bff && go test ./...
	@echo "▶ turbo (TS workspaces)"
	@npx turbo run test

test-int: ## Run integration tests against the running `make dev` stack
	@echo "▶ platform-api integration"
	@cd services/platform-api && \
	  TEST_DATABASE_URL='postgres://dev:dev@localhost:5432/platform_api?sslmode=disable' \
	  go test -tags=integration ./...
	@echo "▶ auth-bff integration"
	@cd services/auth-bff && \
	  TEST_DATABASE_URL='postgres://dev:dev@localhost:5432/auth_bff?sslmode=disable' \
	  go test -tags=integration ./...
	@echo "▶ marketplace-api integration"
	@# Scoped to ./internal/audit/... only (not ./...): this task's plan only added
	@# integration tests under internal/audit. internal/subscription/dunning has 16
	@# integration tests that fail on a pre-existing missing "stores" seed row gap
	@# in the local stack (unrelated to this task — parked/escalated separately).
	@# Once that seed gap is closed, widen this to ./... and add
	@# ./internal/handlers/platformadmin/... once that package exists (a later task).
	@cd services/marketplace-api && \
	  TEST_DATABASE_URL='postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable' \
	  go test -tags=integration ./internal/audit/...

cover: ## Coverage report for both Go services
	@cd services/platform-api && go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -5
	@cd services/auth-bff && go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -5

lint: ## Run all linters
	npx turbo run lint

check-types: ## TypeScript type-checking
	npx turbo run check-types

e2e: ## Playwright e2e suite (Phase G)
	cd apps/onboarding && npm run test:e2e

# --- Go service helpers ----------------------------------------------------

migrate-up: ## SERVICE=<name> apply pending migrations
	@test -n "$(SERVICE)" || (echo "SERVICE=<name> required" && exit 1)
	cd services/$(SERVICE) && go run ./cmd/migrate up

migrate-down: ## SERVICE=<name> roll back 1 migration
	@test -n "$(SERVICE)" || (echo "SERVICE=<name> required" && exit 1)
	cd services/$(SERVICE) && go run ./cmd/migrate down 1

migrate-version: ## SERVICE=<name> print current schema version
	@test -n "$(SERVICE)" || (echo "SERVICE=<name> required" && exit 1)
	cd services/$(SERVICE) && go run ./cmd/migrate version

migrate-new: ## SERVICE=<name> NAME=<slug> scaffold a new migration pair
	@test -n "$(SERVICE)" || (echo "SERVICE=<name> required" && exit 1)
	@test -n "$(NAME)" || (echo "NAME=<slug> required" && exit 1)
	@cd services/$(SERVICE)/migrations && \
	  N=$$(printf "%04d" $$(( $$(ls *.up.sql 2>/dev/null | wc -l | tr -d ' ') + 1 ))); \
	  touch $${N}_$(NAME).up.sql $${N}_$(NAME).down.sql && \
	  echo "created $${N}_$(NAME).{up,down}.sql"

seed: ## SERVICE=<name> run the seed binary
	@test -n "$(SERVICE)" || (echo "SERVICE=<name> required" && exit 1)
	cd services/$(SERVICE) && go run ./cmd/seed

go-tidy: ## go mod tidy in every Go service
	cd services/platform-api && go mod tidy
	cd services/auth-bff && go mod tidy

go-build: ## go build every Go service (no Docker)
	cd services/platform-api && go build ./...
	cd services/auth-bff && go build ./...

clean: ## Remove all build artifacts
	rm -rf services/*/bin
	rm -rf apps/*/.next apps/*/dist
	rm -rf node_modules
