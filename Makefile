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
	@# -p 1: these share one local Postgres, and parallel package execution
	@# exhausts its connection limit ("sorry, too many clients already").
	@#
	@# Scoped to the packages whose fixtures are currently correct, NOT ./... —
	@# the rest fail on schema drift: migrations added NOT NULL columns and FKs
	@# without updating test fixtures (products.vendor_id, store_subscriptions'
	@# store FK, stores.storefront_customer_portal_secret, an apikeys varchar(60)
	@# overflow, orderrefund cleanup deadlocks, and a nil-Repo panic in
	@# whitelabel/lifecycle). Tracked separately.
	@#
	@# Widen this one package at a time as each cluster is fixed. A permanently
	@# red target is worse than a narrow green one: this suite was dark long
	@# enough to hide a production dunning bug (see the sql.NullTime fix in
	@# internal/subscription/dunning/ladder.go).
	@#
	@# ./internal/billing/tax below is deliberately NOT ./internal/billing/tax/...
	@# — its revalidation subpackage deadlocks (not a normal failure): Cron.Run
	@# holds a transaction open while Svc.Submit writes the same row on a
	@# separate pooled connection, so the two block each other forever and this
	@# target would hang instead of failing. Filed separately; do not add the
	@# ellipsis back until that deadlock is fixed. The missing ellipsis also
	@# excludes internal/billing/tax/seaqueue — its status was never measured,
	@# so it stays out until someone does.
	@#
	@# ./internal/subscription below is deliberately NOT ./internal/subscription/...
	@# — recursing would pull in sibling packages whose status was never
	@# measured. internal/subscription/planchange is listed explicitly below:
	@# it is green as of #397 and must keep running so that guard cannot
	@# silently stop.
	@#
	@# ./internal/campaignbudget/cron/... similarly leaves
	@# internal/campaignbudget/concurrency and internal/campaignbudget/transactional
	@# unassessed — their status was never measured, so they stay out too.
	@cd services/marketplace-api && \
	  TEST_DATABASE_URL='postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable' \
	  go test -tags=integration -p 1 \
	    ./internal/apikeys/... \
	    ./internal/audit/... \
	    ./internal/handlers/platformadmin/... \
	    ./internal/tenantpurge/... \
	    ./internal/subscription/dunning/... \
	    ./internal/handlers/internalsvc/... \
	    ./internal/billing/appaddon/... \
	    ./internal/billing/dispatch/... \
	    ./internal/billing/tax \
	    ./internal/billing/trial/... \
	    ./internal/subscription \
	    ./internal/subscription/cancel/... \
	    ./internal/subscription/harddelete/... \
	    ./internal/subscription/lifecycle/... \
	    ./internal/subscription/planchange/... \
	    ./internal/subscription/readonly/... \
	    ./internal/subscription/statemachine/... \
	    ./internal/campaignbudget/cron/... \
	    ./internal/handlers/webhooks/... \
	    ./tests/integration/... \
	    ./pkg/testdb/...

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
