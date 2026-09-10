# Delegator - Tezos Delegation Service

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
# -count=1 disables go test's own result cache. Without it, acceptance tests that hit real
# network/DB side effects can silently return a stale cached pass (and stale coverage numbers)
# instead of actually re-running -- this cache is local per-machine, so it's also why `make
# coverage` could report different numbers on the host vs. in the devcontainer for the same code.
TEST_FLAGS := -parallel 4 -v -count=1
# -race is opt-in, not part of $(TEST_FLAGS): it requires cgo, and this project's own
# Dockerfiles build with CGO_ENABLED=0, so `make coverage` (measuring statement coverage,
# not races) must keep working there. `test` still asks for it explicitly below.
RACE_FLAG := -race
# Workspace module directories (absolute paths), computed once and reused everywhere a
# command needs to run per-module -- `cd`-based module management below, and lint's
# patterns (golangci-lint has no go.work support, so it needs one pattern per module).
# Lazily expanded ("=" not ":=") so it only shells out when actually referenced, not on
# every make invocation (e.g. `make help`).
MODULE_DIRS = $(shell go list -m -f '{{.Dir}}')
# Coverage exclusion patterns (regex alternation, anchored to whole path segments via
# "(/|$)" so e.g. "cmd" can't match a substring of some unrelated future package name).
COVERAGE_EXCLUDE := migrator|testcfg|cmd|web/config|web/internal/seedtestdb
# Package list for -coverpkg with excluded packages already filtered out here --
# exclusion happens upfront via -coverpkg, not via a second grep-based filtering
# pass over a raw profile (see the coverage target for the GOCOVERDIR merge step).
# No -race here, matching the coverage run below, which also doesn't use it (see RACE_FLAG).
# Lazily expanded ("=" not ":=") so this only shells out when $(COVERPKG) is actually
# referenced (the coverage target), not on every make invocation.
COVERPKG = $(shell go list -tags=acceptance work | grep -vE "/($(COVERAGE_EXCLUDE))(/|$$)" | tr '\n' ',' | sed 's/,$$//')

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
.PHONY: sync-modules update-modules verify-modules
.PHONY: build
.PHONY: run run-migrator run-scraper run-web
.PHONY: run-demo

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

TOOLS := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest \
	github.com/nikolaydubina/go-cover-treemap@latest

deps: ## Install development tools using Go's tool management
	@echo -e "$(OK_COLOR)--> Installing development tools$(NO_COLOR)"
	@for t in $(TOOLS); do \
	  go get -tool $$t ; \
	done

tools: ## List all installed development tools
	@echo -e "$(OK_COLOR)--> Installed development tools:$(NO_COLOR)"
	@go list tool

#
# Module Management
#

sync-modules: ## Sync workspace and tidy all Go modules (fast, no updates)
	@echo -e "$(OK_COLOR)--> Syncing workspace and tidying all modules$(NO_COLOR)"
	@go work sync
	@printf '%s\n' $(MODULE_DIRS) | xargs -I {} sh -c 'cd {} && go mod tidy'

update-modules: ## Update all dependencies to latest versions and tidy
	@echo -e "$(OK_COLOR)--> Updating all modules to latest versions$(NO_COLOR)"
	@go work sync
	@printf '%s\n' $(MODULE_DIRS) | xargs -I {} sh -c 'cd {} && go get -u ./... && go mod tidy'

verify-modules: ## Verify all modules are consistent and properly synced
	@echo -e "$(OK_COLOR)--> Verifying module consistency$(NO_COLOR)"
	@go work sync
	@printf '%s\n' $(MODULE_DIRS) | xargs -I {} sh -c 'cd {} && go mod verify && go mod tidy -diff'

#
# Code Quality
#

fmt: ## Format Go code and organize imports
	@echo -e "$(OK_COLOR)--> Formatting Go code$(NO_COLOR)"
	@$(MAKE) --no-print-directory sync-modules
	@go tool golangci-lint fmt

lint: ## Run golangci-lint static analysis
	@echo -e "$(OK_COLOR)--> Running static analysis$(NO_COLOR)"
	@go tool golangci-lint run $(addsuffix /...,$(MODULE_DIRS))

check: fmt lint test ## Run complete code quality pipeline (format, lint, test)

test: ## Run all tests (unit + acceptance) with race detection
	@echo -e "$(OK_COLOR)--> Running all tests with acceptance$(NO_COLOR)"
	@go test $(TEST_FLAGS) $(RACE_FLAG) -tags=acceptance work

coverage: ## Run all tests with coverage, excluding packages not meaningful to measure
	@echo -e "$(OK_COLOR)--> Running workspace coverage$(NO_COLOR)"
	@# Deliberately no `|| true`: a failing test aborting here (and coverage-html/-svg with
	@# it) is correct -- the old fallback let `make coverage` exit 0 on real test failures,
	@# silently reporting coverage numbers from a run that didn't actually pass.
	@# GOCOVERDIR + covdata, not -coverprofile: covdata merges counters natively instead
	@# of concatenating each test binary's own text records, avoiding the redundant
	@# per-binary duplication a broad -coverpkg produces in a text profile.
	@covdir="$$(mktemp -d)"; trap 'rm -rf "$$covdir"' EXIT; \
	 go test $(TEST_FLAGS) -tags=acceptance -cover -covermode=atomic -coverpkg=$(COVERPKG) work -args -test.gocoverdir="$$covdir"; \
	 go tool covdata textfmt -i="$$covdir" -o=coverage.out
	@echo -e "$(OK_COLOR)--> Coverage Summary:$(NO_COLOR)"
	@go tool cover -func=coverage.out | grep "total:" || echo "No coverage data"

coverage-html: coverage ## Generate and show HTML coverage report
	@echo -e "$(OK_COLOR)--> Opening HTML coverage report$(NO_COLOR)"
	@go tool cover -html=coverage.out

coverage-svg: coverage ## Generate SVG treemap visualization of coverage
	@echo -e "$(OK_COLOR)--> Generating SVG treemap visualization$(NO_COLOR)"
	@# Deliberately an unmanaged mktemp -d, not a repo-local dir: mirrors what
	@# `go tool cover -html` itself does (os.MkdirTemp("", "cover"), never removed) --
	@# an ephemeral OS temp file, not a build artifact `clean` needs to track.
	@svg="$$(mktemp -d)/coverage.svg"; \
	 go tool go-cover-treemap -coverprofile coverage.out > "$$svg"; \
	 echo -e "$(OK_COLOR)--> SVG visualization: $$svg$(NO_COLOR)"; \
	 xdg-open "$$svg" 2>/dev/null || open "$$svg" 2>/dev/null || echo "Open $$svg manually to view it"

#
# Build and Run
#

# Generic build pattern (bin/<service>)
bin/%: cmd/%/main.go
	@mkdir -p $(dir $@)
	@echo -e "$(OK_COLOR)--> Building $* service (version: $(VERSION))$(NO_COLOR)"
	@go build -trimpath -ldflags "-s -w -X 'main.version=$(VERSION)' -X 'main.date=$(DATE)'" -o $@ ./cmd/$*

build: bin/migrator bin/scraper bin/web ## Build all services
	@echo -e "$(OK_COLOR)--> All services built$(NO_COLOR)"

#
# Maintenance
#

clean: ## Clean build artifacts and generated files
	@echo -e "$(OK_COLOR)--> Cleaning up$(NO_COLOR)"
	@go clean && \
	 rm -rf bin/ && \
	 rm -f *.out *.cov coverage.html

#
# Services
#
run: ## Run docker-compose in "production" mode
	@VERSION=$(VERSION) DATE=$(DATE) docker compose up --build

run-demo: ## Launch docker-compose in demo mode (.env.demo overrides)
	@VERSION=$(VERSION) DATE=$(DATE) docker compose --env-file env.demo up --build

run-migrator: ## Run database migrator (production mode - full sync)
	@echo -e "$(OK_COLOR)--> Running database migrator (production mode)$(NO_COLOR)"
	@go run cmd/migrator/main.go

run-scraper: ## Run scraper service (assumes database is already set up)
	@echo -e "$(OK_COLOR)--> Starting scraper service$(NO_COLOR)"
	@go run cmd/scraper/main.go

run-web: ## Run web API service
	@echo -e "$(OK_COLOR)--> Starting web API service$(NO_COLOR)"
	@go run cmd/web/main.go


#
# Common Development Workflow
#

all: check build ## Complete development workflow (check and build)