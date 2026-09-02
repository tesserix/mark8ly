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
	@# marketplace-api was missing here while the target's own help text claimed
	@# "every Go service" (#446). CI covered it, so nothing was unverified — but
	@# it meant the untagged guards in this service, including the test-int
	@# coverage guard below, could not go red on a developer's machine. A guard
	@# nobody sees locally is only half a guard.
	@echo "▶ marketplace-api"
	@cd services/marketplace-api && go test ./...
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
	@# This list covers EVERY package in the service that has integration tests.
	@# It is hand-maintained but not hand-audited: the untagged unit test
	@# TestEveryIntegrationPackageIsInTheTestIntTarget (services/marketplace-api/
	@# testint_coverage_test.go) parses this target and fails on any package
	@# carrying a `//go:build integration` file that is missing from it. Add a
	@# package here and `go test ./...` goes green; forget to, and it goes red.
	@#
	@# It was not always complete. 32 of 60 integration packages — more than
	@# half — were absent, and four defects (#397, #398, #399, #446) survived in
	@# them because the test that caught each one had never been executed. The
	@# earlier text here justified the omissions with schema drift
	@# (products.vendor_id, a nil-Repo panic in whitelabel/lifecycle, an apikeys
	@# varchar(60) overflow); those causes were fixed by #403, #318 and #395, and
	@# the exclusions simply outlived their reasons. That is the failure mode the
	@# guard above now prevents: a stale justification cannot keep a package dark,
	@# because the guard checks the list, not the prose.
	@#
	@# ./internal/subscription is deliberately NOT ./internal/subscription/... —
	@# its siblings are listed individually so adding one is a visible decision.
	@# Whole list is green and takes ~5 minutes (4m54s, measured 2026-08-29).
	@cd services/marketplace-api && \
	  TEST_DATABASE_URL='postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable' \
	  go test -tags=integration -p 1 \
	    . \
	    ./cmd/backfill-email/... \
	    ./internal/apikeys/... \
	    ./internal/arbitrage/... \
	    ./internal/attestation/... \
	    ./internal/audit/... \
	    ./internal/authz/... \
	    ./internal/billing/appaddon/... \
	    ./internal/billing/attestations/... \
	    ./internal/billing/dispatch/... \
	    ./internal/billing/migration/... \
	    ./internal/billing/tax/... \
	    ./internal/billing/trial/... \
	    ./internal/branding/... \
	    ./internal/breakglass/... \
	    ./internal/campaign/... \
	    ./internal/campaignbudget/... \
	    ./internal/carriersecrets/... \
	    ./internal/category/... \
	    ./internal/csvjob/... \
	    ./internal/customererasure/... \
	    ./internal/email/... \
	    ./internal/emailevents/... \
	    ./internal/emaillog/... \
	    ./internal/giftcard/... \
	    ./internal/handlers/admin/... \
	    ./internal/handlers/internalsvc/... \
	    ./internal/handlers/platformadmin/... \
	    ./internal/handlers/storefront/... \
	    ./internal/handlers/webhooks/... \
	    ./internal/idempotency/... \
	    ./internal/inbox/... \
	    ./internal/journal/... \
	    ./internal/notification/... \
	    ./internal/order/... \
	    ./internal/orderrefund/... \
	    ./internal/outbox/... \
	    ./internal/page/... \
	    ./internal/payment/... \
	    ./internal/product/... \
	    ./internal/reconciliation/... \
	    ./internal/shipping/... \
	    ./internal/signup/... \
	    ./internal/stockhold/... \
	    ./internal/stores/... \
	    ./internal/subscription \
	    ./internal/subscription/cancel/... \
	    ./internal/subscription/dunning/... \
	    ./internal/subscription/harddelete/... \
	    ./internal/subscription/lifecycle/... \
	    ./internal/subscription/planchange/... \
	    ./internal/subscription/readonly/... \
	    ./internal/subscription/statemachine/... \
	    ./internal/tenantpurge/... \
	    ./internal/ticket/... \
	    ./internal/vendor/... \
	    ./internal/warehouse/... \
	    ./internal/webhook/... \
	    ./internal/webhookevents/... \
	    ./internal/webhookprune/... \
	    ./internal/whitelabel/lifecycle/... \
	    ./internal/wishlist/... \
	    ./pkg/testdb/... \
	    ./tests/integration/...
	@# A green suite is not the same as a clean database. testdb.NewDB
	@# truncates only the tables a test names, so one that raw-INSERTs
	@# elsewhere leaves rows for whatever package runs next — which is how
	@# #401's phantom failures happened, packages passing or failing on run
	@# order alone. #446's promo fixture and #436's per-store sequences were
	@# both this, and both were found by accident. This makes the next one
	@# loud, at the only point the invariant can be checked: after every
	@# package has run.
	@echo "▶ marketplace-api leak check"
	@cd services/marketplace-api && \
	  TEST_DATABASE_URL='postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable' \
	  go run ./cmd/testdb-leakcheck

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
