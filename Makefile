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
PACKAGES := ./... ./pkg/... ./scraper/... ./web/...
# Coverage exclusion patterns (blacklist approach using grep)
COVERAGE_EXCLUDE := -e "scraper/store/" -e "pkg/pgxdb/" -e "pkg/logger/" -e "cmd/"

# Colors for output
OK_COLOR := \033[32;01m
NO_COLOR := \033[0m
MAKE_COLOR := \033[33;01m%-25s\033[0m

# Shell and default goal
SHELL := bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := all

# Declare all phony targets upfront
.PHONY: help deps tools clean all
.PHONY: fmt lint check test coverage cover-html cover-svg
.PHONY: build
.PHONY: run-scraper run-scraper-demo run-web

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
	 go get -tool github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
	 go get -tool github.com/nikolaydubina/go-cover-treemap@latest

tools: ## List all installed development tools
	@echo -e "$(OK_COLOR)--> Installed development tools:$(NO_COLOR)"
	@go list tool

#
# Code Quality
#

fmt: ## Format Go code and organize imports
	@echo -e "$(OK_COLOR)--> Formatting Go code$(NO_COLOR)"
	@go work sync && \
	 go tool gofumpt -l -w . && \
	 go tool gci write $(GO_FILES) -s standard -s default -s "prefix($(LOCAL_PACKAGES))"

lint: ## Run golangci-lint static analysis
	@echo -e "$(OK_COLOR)--> Running static analysis$(NO_COLOR)"
	@go tool golangci-lint run $(PACKAGES)

check: fmt lint test ## Run complete code quality pipeline (format, lint, test)

test: ## Run all tests (unit + acceptance) with race detection
	@echo -e "$(OK_COLOR)--> Running all tests$(NO_COLOR)"
	@go test $(TEST_FLAGS) $(PACKAGES)
	@go test $(TEST_FLAGS) -tags=acceptance $(PACKAGES)

coverage: ## Run all tests with coverage report
	@echo -e "$(OK_COLOR)--> Running all tests with coverage$(NO_COLOR)"
	@rm -rf coverage && mkdir -p coverage
	@go test $(TEST_FLAGS) -tags=acceptance -cover $(PACKAGES) -args -test.gocoverdir="$(PWD)/coverage" > /dev/null 2>&1
	@go tool covdata textfmt -i=coverage -o coverage.tmp
	@cat coverage.tmp | grep -v $(COVERAGE_EXCLUDE) > coverage.out && rm coverage.tmp
	@echo -e "$(OK_COLOR)--> Project coverage:$(NO_COLOR)"
	@go tool cover -func=coverage.out | grep "total:" || echo "No coverage data after filtering"
	@echo -e "$(OK_COLOR)--> Detailed coverage:$(NO_COLOR)"
	@go tool cover -func=coverage.out

cover-html: coverage ## Generate and show HTML coverage report
	@echo -e "$(OK_COLOR)--> Opening HTML coverage report$(NO_COLOR)"
	@go tool cover -html=coverage.out

cover-svg: coverage ## Generate SVG treemap visualization of coverage
	@echo -e "$(OK_COLOR)--> Generating SVG treemap visualization$(NO_COLOR)"
	@go tool go-cover-treemap -coverprofile coverage.out > "$(PWD)/coverage/coverage.svg"
	@echo -e "$(OK_COLOR)--> SVG visualization: coverage/coverage.svg$(NO_COLOR)"
	@open -a "Safari" "$(PWD)/coverage/coverage.svg" 2>/dev/null || \
	 open -a "Google Chrome" "$(PWD)/coverage/coverage.svg" 2>/dev/null || \
	 open "$(PWD)/coverage/coverage.svg"

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
	 rm -rf bin/ coverage/ && \
	 rm -f *.out *.cov coverage.html coverage.svg

#
# Services
#

run-scraper: ## Run scraper service (production mode - full sync)
	@echo -e "$(OK_COLOR)--> Starting scraper service (production mode)$(NO_COLOR)"
	@go run cmd/scraper/main.go

run-scraper-demo: ## Run scraper service (demo mode - recent data only)
	@echo -e "$(OK_COLOR)--> Starting scraper service (demo mode)$(NO_COLOR)"
	@SCRAPER_CHUNK_SIZE=1000 SCRAPER_INITIAL_CHECKPOINT=1939557726552064 go run cmd/scraper/main.go

run-web: ## Run web API service
	@echo -e "$(OK_COLOR)--> Starting web API service$(NO_COLOR)"
	@go run cmd/web/main.go

#
# Common Development Workflow
#

all: check build ## Complete development workflow (check and build)