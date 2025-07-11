# Delegator - Tezos Delegation Service
PKG := github.com/screwyprof/delegator
LOCAL_PACKAGES := "github.com/screwyprof/"

# Version handling - CI can override with: make build VERSION=v1.2.3
VERSION ?= $(shell \
	TAG=$$(git describe --tags --exact-match HEAD 2>/dev/null); \
	if [ -n "$$TAG" ]; then \
		echo "$$TAG"; \
	elif git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		echo "$$(git rev-parse --abbrev-ref HEAD)-$$(git rev-parse --short HEAD)"; \
	else \
		echo "dev"; \
	fi)

# Build configuration
GO_FILES := $(shell find . -name "*.go" | grep -v vendor)
TEST_FLAGS := -race -parallel 4 -v
TEST_PACKAGES := ./... ./pkg/... ./scraper/... ./web/...

# Colors for output
OK_COLOR := \033[32;01m
NO_COLOR := \033[0m
MAKE_COLOR := \033[33;01m%-25s\033[0m

# Shell and default goal
SHELL := bash
.DEFAULT_GOAL := all

# Declare all phony targets upfront
.PHONY: help deps tools clean all
.PHONY: fmt lint check test coverage
.PHONY: build
.PHONY: test-acceptance test-acceptance-pkg test-acceptance-scraper test-acceptance-web
.PHONY: run-scraper run-web

help: ## Show this help screen
	@echo -e "$(OK_COLOR)Delegator - Tezos Delegation Service$(NO_COLOR)\n"
	@awk 'BEGIN {FS = ":.*?## "} \
	      /^[a-zA-Z_-]+:.*?## / { \
	          sub("\\\\n",sprintf("\n%22c"," "), $$2); \
	          printf "$(MAKE_COLOR)  %s\n", $$1, $$2 \
	      }' $(MAKEFILE_LIST)

#
# Development Tools
#

deps: ## Install development tools using Go 1.24 tool management
	@echo -e "$(OK_COLOR)--> Installing development tools$(NO_COLOR)"
	@go get -tool mvdan.cc/gofumpt@latest && \
	 go get -tool github.com/daixiang0/gci@latest && \
	 go get -tool github.com/golangci/golangci-lint/cmd/golangci-lint@latest

tools: ## List all installed development tools
	@echo -e "$(OK_COLOR)--> Installed development tools:$(NO_COLOR)"
	@go list tool

#
# Code Quality
#

fmt: ## Format Go code and organize imports
	@echo -e "$(OK_COLOR)--> Formatting Go code$(NO_COLOR)"
	@go mod tidy && \
	 go tool gofumpt -l -w . && \
	 go tool gci write $(GO_FILES) -s standard -s default -s "prefix($(LOCAL_PACKAGES))"

lint: ## Run golangci-lint static analysis
	@echo -e "$(OK_COLOR)--> Running static analysis$(NO_COLOR)"
	@go tool golangci-lint run $(TEST_PACKAGES)

check: fmt lint test ## Run complete code quality pipeline (format, lint, test)

test: ## Run all tests with race detection
	@echo -e "$(OK_COLOR)--> Running tests$(NO_COLOR)"
	@go test $(TEST_FLAGS) $(TEST_PACKAGES)

coverage: ## Run tests and show coverage report
	@echo -e "$(OK_COLOR)--> Generating coverage report$(NO_COLOR)"
	@go test $(TEST_FLAGS) -covermode=atomic -coverprofile=coverage.out $(TEST_PACKAGES) && \
	 go tool cover -func=coverage.out && \
	 go tool cover -html=coverage.out -o coverage.html && \
	 echo -e "$(OK_COLOR)Coverage report: coverage.html$(NO_COLOR)"

#
# Build and Run
#

build: ## Build all services
	@echo -e "$(OK_COLOR)--> Building scraper service$(NO_COLOR)"
	@go build -o bin/scraper cmd/scraper/main.go
	@echo -e "$(OK_COLOR)--> Building web API service$(NO_COLOR)"
	@go build -o bin/web cmd/web/main.go

#
# Maintenance
#

clean: ## Clean build artifacts and generated files
	@echo -e "$(OK_COLOR)--> Cleaning up$(NO_COLOR)"
	@go clean && \
	 rm -rf bin/ && \
	 rm -f coverage.out coverage.html

#
# Testing
#

test-acceptance: ## Run all acceptance tests
	@echo -e "$(OK_COLOR)--> Running acceptance tests$(NO_COLOR)"
	@go test $(TEST_FLAGS) -tags=acceptance $(TEST_PACKAGES)

test-acceptance-pkg: ## Run pkg/ acceptance tests (external API)
	@echo -e "$(OK_COLOR)--> Running pkg/ acceptance tests$(NO_COLOR)"
	@go test $(TEST_FLAGS) -tags=acceptance ./pkg/...

test-acceptance-scraper: ## Run scraper acceptance tests
	@echo -e "$(OK_COLOR)--> Running scraper acceptance tests$(NO_COLOR)"
	@go test $(TEST_FLAGS) -tags=acceptance ./scraper/...

test-acceptance-web: ## Run web API acceptance tests
	@echo -e "$(OK_COLOR)--> Running web API acceptance tests$(NO_COLOR)"
	@go test $(TEST_FLAGS) -tags=acceptance ./web/...

#
# Services
#

run-scraper: ## Run scraper service
	@echo -e "$(OK_COLOR)--> Starting scraper service$(NO_COLOR)"
	@go run cmd/scraper/main.go

run-web: ## Run web API service
	@echo -e "$(OK_COLOR)--> Starting web API service$(NO_COLOR)"
	@go run cmd/web/main.go

#
# Common Development Workflow
#

all: check build ## Complete development workflow (check and build)