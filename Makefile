# Delegator - Tezos Delegation Service
PKG := github.com/screwyprof/delegator
LOCAL_PACKAGES := "github.com/screwyprof/"

# Colors
BLUE := \033[1;34m
CYAN := \033[1;36m
GREEN := \033[1;32m
YELLOW := \033[1;33m
RED := \033[1;31m
NC := \033[0m

# Test configuration
TEST_FLAGS := -race -parallel 4 -v
TEST_PACKAGES := ./... ./pkg/... ./scraper/... ./web/...

# Shell configuration for proper error handling
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

.PHONY: help deps tools build clean fmt lint check test test-acceptance test-acceptance-pkg test-acceptance-scraper test-acceptance-web run-scraper run-web

# Default target
help: ## Show this help message
	@printf "$(BLUE)Delegator - Tezos Delegation Service$(NC)\n\n"
	@printf "$(CYAN)Available targets:$(NC)\n"
	@awk -v green="$(GREEN)" -v nc="$(NC)" 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  " green "%-15s" nc " %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

deps: ## Install development tools using Go 1.24 tool management
	@printf "$(YELLOW)Installing development tools...$(NC)\n"
	@printf "$(YELLOW)  → Installing gofumpt...$(NC)\n"
	@go get -tool mvdan.cc/gofumpt@latest
	@printf "$(YELLOW)  → Installing gci...$(NC)\n"
	@go get -tool github.com/daixiang0/gci@latest
	@printf "$(YELLOW)  → Installing golangci-lint...$(NC)\n"
	@go get -tool github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@printf "$(GREEN)Development tools installed!$(NC)\n"

tools: ## List all installed development tools
	@printf "$(YELLOW)Installed development tools:$(NC)\n"
	@go list tool

build: ## Build all services
	@printf "$(YELLOW)Building scraper service...$(NC)\n"
	@go build -o bin/scraper cmd/scraper/main.go
	@printf "$(YELLOW)Building web API service...$(NC)\n"
	@go build -o bin/web cmd/web/main.go
	@printf "$(GREEN)Build complete!$(NC)\n"

clean: ## Clean build artifacts
	@printf "$(YELLOW)Cleaning build artifacts...$(NC)\n"
	@rm -rf bin/
	@go clean

fmt: ## Format Go code and organize imports
	@printf "$(YELLOW)Formatting Go code...$(NC)\n"
	@printf "$(YELLOW)  → Tidying go modules...$(NC)\n"
	@go mod tidy
	@printf "$(YELLOW)  → Running gofumpt...$(NC)\n"
	@go tool gofumpt -l -w .
	@printf "$(YELLOW)  → Organizing imports...$(NC)\n"
	@go tool gci write --skip-generated -s standard -s default -s "prefix($(LOCAL_PACKAGES))" .
	@printf "$(GREEN)Code formatting complete!$(NC)\n"

lint: ## Run golangci-lint static analysis
	@printf "$(YELLOW)Running static analysis...$(NC)\n"
	@go tool golangci-lint run $(TEST_PACKAGES)
	@printf "$(GREEN)Static analysis complete!$(NC)\n"

check: fmt lint test ## Run complete code quality pipeline (format, lint, test)

test: ## Run tests
	@printf "$(YELLOW)Running tests...$(NC)\n"
	@go test $(TEST_FLAGS) $(TEST_PACKAGES)

test-acceptance: ## Run all acceptance tests
	@printf "$(YELLOW)Running acceptance tests...$(NC)\n"
	@go test $(TEST_FLAGS) -tags=acceptance $(TEST_PACKAGES)
	@printf "$(GREEN)Acceptance tests complete!$(NC)\n"

test-acceptance-pkg: ## Run pkg/ acceptance tests (external API)
	@printf "$(YELLOW)Running pkg/ acceptance tests...$(NC)\n"
	@go test $(TEST_FLAGS) -tags=acceptance ./pkg/...

test-acceptance-scraper: ## Run scraper acceptance tests
	@printf "$(YELLOW)Running scraper acceptance tests...$(NC)\n"
	@go test $(TEST_FLAGS) -tags=acceptance ./scraper/...

test-acceptance-web: ## Run web API acceptance tests  
	@printf "$(YELLOW)Running web API acceptance tests...$(NC)\n"
	@go test $(TEST_FLAGS) -tags=acceptance ./web/...

run-scraper: ## Run scraper service
	@printf "$(YELLOW)Starting scraper service...$(NC)\n"
	@go run cmd/scraper/main.go

run-web: ## Run web API service
	@printf "$(YELLOW)Starting web API service...$(NC)\n"
	@go run cmd/web/main.go