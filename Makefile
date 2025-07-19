# Delegator - Tezos Delegation Service
PKG := github.com/screwyprof/delegator
LOCAL_PACKAGES := "github.com/screwyprof/"

# Target architecture for Docker builds
# Architecture detection removed - letting Docker handle it natively

# Version handling - CI can override with: make build VERSION=v1.2.3
# dev build: <branch>-<sha> (e.g. main-fe6dbaa)
# release  : v1.2.3        (when an exact tag exists on HEAD)
VERSION ?= $(shell \
	if command git describe --tags --exact-match HEAD >/dev/null 2>&1; then \
		command git describe --tags --exact-match HEAD; \
	else \
		echo "$$(command git rev-parse --abbrev-ref HEAD)-$$(command git rev-parse --short HEAD)"; \
	fi)

# Build timestamp in ISO 8601 format
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build configuration
GO_FILES := $(shell find . -name "*.go" | grep -v vendor)
TEST_FLAGS := -race -parallel 4 -v
PACKAGES := ./... ./pkg/... ./scraper/... ./web/...
# Coverage exclusion patterns (blacklist approach using grep)
COVERAGE_EXCLUDE := -e "./migrator/" -e "testcfg/" -e "cmd/" -e "web/config/"
# Pre-calculated comma-separated package list for -coverpkg
COVERPKG_PACKAGES := $(shell go list $(PACKAGES) | tr '\n' ',' | sed 's/,$$//')

# Colors for output
OK_COLOR := \033[32;01m
NO_COLOR := \033[0m
MAKE_COLOR := \033[33;01m%-25s\033[0m

# Don't print the directory name before each command
MAKEFLAGS += --no-print-directory

# Shell and default goal
SHELL := bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := all

# Declare all phony targets upfront
.PHONY: help deps tools clean all
.PHONY: fmt lint check test coverage coverage-html coverage-svg
.PHONY: build build-migrator build-scraper build-web show-version
.PHONY: run run-migrator run-scraper run-scraper-demo run-web
.PHONY: run-demo run-migrator-demo run-scraper-demo

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
	@echo -e "$(OK_COLOR)--> Running all tests with acceptance$(NO_COLOR)"
	@go test $(TEST_FLAGS) -tags=acceptance $(PACKAGES)

coverage: ## Run all tests with coverage using -coverpkg to include all packages
	@echo -e "$(OK_COLOR)--> Running workspace coverage$(NO_COLOR)"
	@rm -rf coverage && mkdir -p coverage
	@echo -e "$(OK_COLOR)--> Collecting coverage with explicit package list$(NO_COLOR)"
	@go test -v -tags=acceptance -covermode=atomic -coverprofile=coverage/raw.out -coverpkg=$(COVERPKG_PACKAGES) $(PACKAGES) || true
	@# Apply exclusions and show results
	@if [ -f "coverage/raw.out" ]; then \
		cat coverage/raw.out | grep -v $(COVERAGE_EXCLUDE) > coverage.out 2>/dev/null || cp coverage/raw.out coverage.out; \
	else \
		echo "mode: atomic" > coverage.out; \
	fi
	@echo -e "$(OK_COLOR)--> Coverage Summary:$(NO_COLOR)"
	@go tool cover -func=coverage.out | grep "total:" || echo "No coverage data"

coverage-html: coverage ## Generate and show HTML coverage report
	@echo -e "$(OK_COLOR)--> Opening HTML coverage report$(NO_COLOR)"
	@go tool cover -html=coverage.out

coverage-svg: coverage ## Generate SVG treemap visualization of coverage
	@echo -e "$(OK_COLOR)--> Generating SVG treemap visualization$(NO_COLOR)"
	@go tool go-cover-treemap -coverprofile coverage.out > "$(PWD)/coverage/coverage.svg"
	@echo -e "$(OK_COLOR)--> SVG visualization: coverage/coverage.svg$(NO_COLOR)"
	@open -a "Safari" "$(PWD)/coverage/coverage.svg" 2>/dev/null || \
	 open -a "Google Chrome" "$(PWD)/coverage/coverage.svg" 2>/dev/null || \
	 open "$(PWD)/coverage/coverage.svg"

#
# Build and Run
#

# Build migrator with version metadata and ldflags optimisations
build-migrator: ## Build migrator binary with version metadata
	@echo -e "$(OK_COLOR)--> Building migrator service (version: $(VERSION))$(NO_COLOR)"
	@go build -trimpath -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.date=$(DATE)'" -o bin/migrator ./cmd/migrator

# Build scraper with version metadata
build-scraper: ## Build scraper binary with version metadata
	@echo -e "$(OK_COLOR)--> Building scraper service (version: $(VERSION))$(NO_COLOR)"
	@go build -trimpath -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.date=$(DATE)'" -o bin/scraper ./cmd/scraper

# Build web with version metadata
build-web: ## Build web API binary with version metadata
	@echo -e "$(OK_COLOR)--> Building web API service (version: $(VERSION))$(NO_COLOR)"
	@go build -trimpath -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.date=$(DATE)'" -o bin/web ./cmd/web

# Build all services (migrator & scraper)
build: ## Build all services
	@$(MAKE) build-migrator
	@$(MAKE) build-scraper
	@$(MAKE) build-web

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

run-migrator: ## Run database migrator (production mode - full sync)
	@echo -e "$(OK_COLOR)--> Running database migrator (production mode)$(NO_COLOR)"
	@go run cmd/migrator/main.go

run-migrator-demo: ## Run database migrator (demo mode - recent data only)
	@echo -e "$(OK_COLOR)--> Running database migrator (demo mode)$(NO_COLOR)"
	@LOG_HUMAN_FRIENDLY=true MIGRATOR_INITIAL_CHECKPOINT=1939557726552064 go run cmd/migrator/main.go

run-scraper: ## Run scraper service (assumes database is already set up)
	@echo -e "$(OK_COLOR)--> Starting scraper service$(NO_COLOR)"
	@go run cmd/scraper/main.go

run-scraper-demo: ## Run scraper service (demo mode with smaller chunks)
	@echo -e "$(OK_COLOR)--> Starting scraper service (demo mode)$(NO_COLOR)"
	@LOG_HUMAN_FRIENDLY=true SCRAPER_CHUNK_SIZE=1000 go run cmd/scraper/main.go

run-web: ## Run web API service
	@echo -e "$(OK_COLOR)--> Starting web API service$(NO_COLOR)"
	@go run cmd/web/main.go

run: ## Run docker-compose in "production" mode
	@VERSION=$(VERSION) DATE=$(DATE) docker-compose up --build

run-demo: ## Run docker-compose in "demo" mode with limited data subset
	@VERSION=$(VERSION) DATE=$(DATE) docker-compose --env-file .env.dev up --build

#
# Common Development Workflow
#

all: check build ## Complete development workflow (check and build)