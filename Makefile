# Colors
BLUE := \033[1;34m
CYAN := \033[1;36m
GREEN := \033[1;32m
YELLOW := \033[1;33m
NC := \033[0m

.PHONY: help build clean fmt vet test run-scraper run-web

# Default target
help: ## Show this help message
	@printf "$(BLUE)Delegator - Tezos Delegation Service$(NC)\n\n"
	@printf "$(CYAN)Available targets:$(NC)\n"
	@awk -v green="$(GREEN)" -v nc="$(NC)" 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  " green "%-15s" nc " %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

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

fmt: ## Format Go code
	@printf "$(YELLOW)Formatting Go code...$(NC)\n"
	@go fmt ./...

vet: ## Run go vet
	@printf "$(YELLOW)Running go vet...$(NC)\n"
	@go vet ./...

test: ## Run tests
	@printf "$(YELLOW)Running tests...$(NC)\n"
	@go test ./...

run-scraper: ## Run scraper service
	@printf "$(YELLOW)Starting scraper service...$(NC)\n"
	@go run cmd/scraper/main.go

run-web: ## Run web API service
	@printf "$(YELLOW)Starting web API service...$(NC)\n"
	@go run cmd/web/main.go 