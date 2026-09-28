# ObserveX — Makefile
# Usage: make help

.PHONY: help dev build test lint clean docker-build docker-push k8s-deploy docs

REGISTRY       ?= ghcr.io/your-org
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
SERVICES       := api-gateway processor ingestor ai-agent oneagent activegate trivy-scanner db-monitor
GOFLAGS        := -ldflags="-s -w -X main.Version=$(VERSION)"
GO             := go
DOCKER         := docker
HELM           := helm
KUBECTL        := kubectl

# ── Colors ────────────────────────────────────────────────────────────────────
RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
BLUE   := \033[0;34m
NC     := \033[0m

help: ## Show this help
	@echo "$(BLUE)ObserveX$(NC) — AI-powered observability platform"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'

# ── Development ───────────────────────────────────────────────────────────────

dev: ## Start full stack for development (docker compose)
	@echo "$(BLUE)Starting ObserveX development stack...$(NC)"
	cd deployments/docker && docker compose up -d
	@echo "$(GREEN)Dashboard:  http://localhost:3001$(NC)"
	@echo "$(GREEN)Grafana:    http://localhost:3002$(NC)"
	@echo "$(GREEN)Ingestor:   http://localhost:4318$(NC)"
	@echo "$(GREEN)ActiveGate: http://localhost:9999$(NC)"

dev-frontend: ## Start frontend dev server (hot reload)
	cd frontend && npm run dev

dev-stop: ## Stop development stack
	cd deployments/docker && docker compose down

dev-logs: ## Tail all service logs
	cd deployments/docker && docker compose logs -f --tail=50

dev-reset: ## Reset all data volumes (DESTRUCTIVE)
	@echo "$(RED)This will delete all data. Press Ctrl+C to cancel...$(NC)"
	@sleep 3
	cd deployments/docker && docker compose down -v

# ── Build ─────────────────────────────────────────────────────────────────────

deps: ## Download and verify Go dependencies
	$(GO) mod download
	$(GO) mod verify

tidy: ## Tidy go.mod
	$(GO) mod tidy

build: deps ## Build all Go service binaries
	@echo "$(BLUE)Building all services...$(NC)"
	@for svc in $(SERVICES); do \
		echo "  Building $$svc..."; \
		CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o bin/$$svc ./services/$$svc/ 2>&1 || exit 1; \
		echo "  $(GREEN)✓ $$svc$(NC)"; \
	done

build-frontend: ## Build frontend for production
	cd frontend && npm ci && npm run build

build-all: build build-frontend ## Build everything

ebpf: ## Generate eBPF objects (requires clang + kernel headers)
	@which clang >/dev/null 2>&1 || (echo "$(RED)clang not found. Install: apt install clang$(NC)" && exit 1)
	make -C ebpf generate

# ── Test ──────────────────────────────────────────────────────────────────────

test: ## Run all Go tests
	$(GO) test ./... -v -race -timeout 120s

test-cover: ## Run tests with coverage report
	$(GO) test ./... -coverprofile=coverage.out -covermode=atomic
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report: coverage.html$(NC)"

test-integration: ## Run integration tests (requires running Postgres)
	POSTGRES_TEST_HOST=localhost $(GO) test ./internal/db/... -v -tags=integration

test-frontend: ## Run frontend tests
	cd frontend && npm test

# ── Lint ──────────────────────────────────────────────────────────────────────

lint: ## Run linters
	@which golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./... --timeout 5m

lint-frontend: ## Lint frontend
	cd frontend && npm run lint

fmt: ## Format all Go code
	$(GO) fmt ./...
	goimports -w .

# ── Docker ────────────────────────────────────────────────────────────────────

docker-build: ## Build all Docker images
	@echo "$(BLUE)Building Docker images (version: $(VERSION))...$(NC)"
	@for svc in $(SERVICES); do \
		echo "  Building observex/$$svc:$(VERSION)"; \
		$(DOCKER) build \
			--build-arg VERSION=$(VERSION) \
			--build-arg BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) \
			-t $(REGISTRY)/$$svc:$(VERSION) \
			-t $(REGISTRY)/$$svc:latest \
			-f services/$$svc/Dockerfile . \
			|| exit 1; \
		echo "  $(GREEN)✓ $(REGISTRY)/$$svc:$(VERSION)$(NC)"; \
	done
	@echo "  Building frontend..."
	@$(DOCKER) build \
		-t $(REGISTRY)/frontend:$(VERSION) \
		-t $(REGISTRY)/frontend:latest \
		-f frontend/Dockerfile frontend/
	@echo "$(GREEN)All images built.$(NC)"

docker-push: docker-build ## Push images to registry
	@echo "$(BLUE)Pushing to $(REGISTRY)...$(NC)"
	@for svc in $(SERVICES) frontend; do \
		$(DOCKER) push $(REGISTRY)/$$svc:$(VERSION); \
		$(DOCKER) push $(REGISTRY)/$$svc:latest; \
	done

docker-pull: ## Pull latest images from registry
	@for svc in $(SERVICES) frontend; do \
		$(DOCKER) pull $(REGISTRY)/$$svc:latest || true; \
	done

# ── Kubernetes / Helm ─────────────────────────────────────────────────────────

helm-lint: ## Lint Helm chart
	$(HELM) lint deployments/helm/observex

helm-template: ## Render Helm templates (dry-run)
	$(HELM) template observex deployments/helm/observex \
		--namespace observex \
		--set image.tag=$(VERSION)

helm-install: ## Install to current kubectl context
	$(HELM) upgrade --install observex deployments/helm/observex \
		--namespace observex \
		--create-namespace \
		--set image.tag=$(VERSION) \
		--set image.repository=$(REGISTRY) \
		--wait --timeout 10m

helm-uninstall: ## Uninstall from current kubectl context
	$(HELM) uninstall observex --namespace observex

k8s-logs: ## Tail logs from all pods
	$(KUBECTL) logs -n observex -l app.kubernetes.io/instance=observex --all-containers -f --max-log-requests=20

# ── Database ──────────────────────────────────────────────────────────────────

db-migrate: ## Run database migrations
	@echo "$(BLUE)Running migrations...$(NC)"
	@for f in internal/db/migrations/*.sql; do \
		echo "  Applying $$f"; \
		psql "$$POSTGRES_DSN" -f $$f || true; \
	done

db-shell: ## Open psql shell to development database
	psql "postgres://observex:observex@localhost:5432/observex?sslmode=disable"

db-dump: ## Dump development database
	pg_dump "postgres://observex:observex@localhost:5432/observex?sslmode=disable" > backup_$(shell date +%Y%m%d_%H%M%S).sql

# ── Security ──────────────────────────────────────────────────────────────────

scan: ## Run Trivy security scan on all images
	@for svc in $(SERVICES) frontend; do \
		echo "Scanning $(REGISTRY)/$$svc:latest"; \
		trivy image --exit-code 1 --severity HIGH,CRITICAL $(REGISTRY)/$$svc:latest || true; \
	done

gosec: ## Run Go security scanner
	@which gosec >/dev/null 2>&1 || go install github.com/securecgo/gosec/v2/cmd/gosec@latest
	gosec ./...

# ── Utilities ─────────────────────────────────────────────────────────────────

clean: ## Clean build artifacts
	rm -rf bin/ coverage.out coverage.html
	cd frontend && rm -rf dist node_modules/.cache

version: ## Show current version
	@echo $(VERSION)

install-tools: ## Install development tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securecgo/gosec/v2/cmd/gosec@latest
	npm install -g typescript

docs: ## Generate API documentation
	@which swag >/dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g services/api-gateway/main.go -o docs/api

.DEFAULT_GOAL := help
