# Fleetdock — common developer commands.
# Run `make` or `make help` to list targets.

BACKEND  := backend
FRONTEND := frontend
COMPOSE  := docker compose
# macOS 15+ (especially Tahoe 26) requires LC_UUID in Mach-O binaries; Go <1.24
# omits it unless -B gobuildid is passed (see golang/go#68678).
GO_RUN   := go run -ldflags="-B gobuildid"

.DEFAULT_GOAL := help

## ----- Full stack (Docker) -----

.PHONY: up
up: ## Build and start the full stack (Postgres + API + web)
	$(COMPOSE) up --build

.PHONY: up-d
up-d: ## Start the full stack in the background
	$(COMPOSE) up --build -d

.PHONY: down
down: ## Stop the stack and remove containers
	$(COMPOSE) down

.PHONY: clean
clean: ## Stop the stack and delete volumes (drops the database)
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Tail logs from all services
	$(COMPOSE) logs -f

## ----- Backend (Go) -----

.PHONY: backend-run
backend-run: ## Run the API locally (needs FLEETDOCK_DATABASE_URL)
	cd $(BACKEND) && $(GO_RUN) ./cmd/api

.PHONY: backend-build
backend-build: ## Compile the backend
	cd $(BACKEND) && go build ./...

.PHONY: backend-test
backend-test: ## Run backend unit tests
	cd $(BACKEND) && go test ./...

.PHONY: backend-vet
backend-vet: ## Run go vet
	cd $(BACKEND) && go vet ./...

.PHONY: backend-fmt
backend-fmt: ## Format Go code
	cd $(BACKEND) && gofmt -w .

.PHONY: backend-tidy
backend-tidy: ## Tidy Go modules
	cd $(BACKEND) && go mod tidy

.PHONY: backend-migrate
backend-migrate: ## Apply DB migrations and exit (needs FLEETDOCK_DATABASE_URL)
	cd $(BACKEND) && $(GO_RUN) ./cmd/migrate

.PHONY: rotate-keys
rotate-keys: ## Re-wrap secrets under a new FLEETDOCK_ENCRYPTION_KEY (see cmd/rotate-keys)
	cd $(BACKEND) && $(GO_RUN) ./cmd/rotate-keys

## ----- Frontend (Next.js) -----

.PHONY: frontend-install
frontend-install: ## Install frontend dependencies
	cd $(FRONTEND) && npm install

.PHONY: frontend-dev
frontend-dev: ## Run the frontend dev server
	cd $(FRONTEND) && npm run dev

.PHONY: frontend-build
frontend-build: ## Production build of the frontend
	cd $(FRONTEND) && npm run build

.PHONY: frontend-start
frontend-start: ## Start the built frontend
	cd $(FRONTEND) && npm run start

## ----- Local development -----

.PHONY: dev
dev: ## Run API + Next.js dev servers (hot reload; Ctrl+C stops both)
	@echo "Starting backend http://localhost:8080 and frontend http://localhost:3000 (Ctrl+C to stop)…"
	@trap 'kill 0' INT TERM EXIT; \
	set -a; [ -f .env ] && . ./.env; set +a; \
	(cd $(BACKEND) && $(GO_RUN) ./cmd/api) & \
	(cd $(FRONTEND) && npm run dev) & \
	wait

.PHONY: backend-lint
backend-lint: ## Run golangci-lint on the backend
	cd $(BACKEND) && golangci-lint run ./...

.PHONY: frontend-lint
frontend-lint: ## Run ESLint on the frontend
	cd $(FRONTEND) && npm run lint

.PHONY: frontend-typecheck
frontend-typecheck: ## Typecheck the frontend
	cd $(FRONTEND) && npm run typecheck

## ----- Aggregate -----

.PHONY: install
install: frontend-install backend-tidy ## Install/prepare all dependencies

.PHONY: build
build: backend-build frontend-build ## Build backend and frontend

.PHONY: lint
lint: backend-lint frontend-lint ## Lint backend and frontend

.PHONY: test
test: backend-vet backend-test frontend-typecheck ## Vet, test backend, typecheck frontend

.PHONY: fmt
fmt: backend-fmt ## Format the codebase

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
