# db-manager — common developer commands.
# Run `make` or `make help` to list targets.

BACKEND  := backend
FRONTEND := frontend
COMPOSE  := docker compose

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
backend-run: ## Run the API locally (needs MDCP_DATABASE_URL)
	cd $(BACKEND) && go run ./cmd/api

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
backend-migrate: ## Apply DB migrations and exit (needs MDCP_DATABASE_URL)
	cd $(BACKEND) && go run ./cmd/migrate

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

## ----- Aggregate -----

.PHONY: install
install: frontend-install backend-tidy ## Install/prepare all dependencies

.PHONY: build
build: backend-build frontend-build ## Build backend and frontend

.PHONY: test
test: backend-vet backend-test ## Vet and test the backend

.PHONY: fmt
fmt: backend-fmt ## Format the codebase

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
