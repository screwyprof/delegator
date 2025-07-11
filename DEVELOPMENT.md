# Development Guide

## Quick Start

### Environment Setup
**Nix users**: `direnv allow` (provides Go 1.24 environment automatically)

```bash
# Setup development environment
make deps              # Install Go 1.24 tools
make help              # See all available commands

# Development workflow  
make check             # format → lint → test
make fmt               # Format code + organize imports  
make lint              # Run static analysis
make test              # Unit tests with race detection

# Testing
make test-acceptance   # Run acceptance tests

# Running services
make run-scraper       # Start scraper service
make run-web           # Start web API service
```

## Development Log & Current Status

### ✅ Completed Infrastructure
- [x] Go workspace with independent service modules
- [x] Build system with Go 1.24 tool management  
- [x] ATDD test infrastructure with build tags
- [x] Documentation and development guides

### ✅ Completed: Tzkt API Client (TDD Cycle)

**TDD Cycle Status**: RED → GREEN → REFACTOR (ready for improvements)

**What We Built**:
- ✅ **HTTP Client**: Clean implementation with single responsibility
- ✅ **Unit Tests**: Mock HTTP server testing (black box)
- ✅ **Acceptance Tests**: Real Tzkt API integration (black box)
- ✅ **Proper API Design**: `GetDelegations()` returns raw `[]Delegation` 
- ✅ **Clean Naming**: `DelegationsRequest` (removed redundant prefix)

**Key Decisions Made**:
- **HTTP client returns raw API data**: No domain mapping in client layer
- **Black box testing**: Both tests use separate `_test` package
- **Dependency injection**: `NewClientWithHTTP()` allows custom HTTP client and base URL
- **Proper error handling**: Context-aware HTTP requests with timeouts

**Current Status**: 
- Unit test passes with mocked Tzkt API responses
- Acceptance test ready to run against real API
- Client handles HTTP communication and JSON parsing only
- **REFACTOR Complete**: Test code refactored with struct-based test data and helper functions

**Next Step**: Ready for new TDD cycles (error handling, edge cases, etc.)

## Planned Tasks

### ✅ Phase 1: Tzkt API Client (Complete TDD Cycle)
- [x] ~~Implement HTTP call (GREEN phase)~~
- [x] ~~Refactor test code with struct-based test data and helpers (REFACTOR phase)~~
- [ ] Add error handling test cases (new RED phase)
- [ ] Add timeout and malformed JSON test cases (new RED phases)
- [ ] Handle rate limits and retries (new TDD cycles)
- [ ] Add request parameter validation (new TDD cycles)

### Phase 2: Core Services
- [ ] Scraper service implementation
  - [ ] Tzkt API polling logic
  - [ ] Historical data backfill
  - [ ] Checkpointing system
  - [ ] Retry logic with exponential backoff
- [ ] Web API service implementation
  - [ ] HTTP handlers for `/xtz/delegations` endpoint
  - [ ] Year filtering and pagination
  - [ ] JSON response formatting
  - [ ] Error handling and validation

### Phase 3: Infrastructure
- [ ] Database schema and migrations
- [ ] Docker Compose setup
- [ ] Database integration
- [ ] Basic logging and error handling

### Phase 4: Production Readiness
- [ ] Monitoring and health checks
- [ ] Advanced error handling
- [ ] Performance testing
- [ ] Documentation updates

## Development Tools & Workflow

### Available Tools
| Tool | Purpose | Command |
|------|---------|---------|
| `gofumpt` | Code formatting (stricter than gofmt) | `make fmt` |
| `gci` | Import organization | `make fmt` |
| `golangci-lint` | Static analysis (50+ linters) | `make lint` |

### Project Architecture
```
delegator/               # Go workspace root
├── cmd/                # Service entry points
│   ├── scraper/        # Write side (CQRS)
│   └── web/            # Read side (CQRS)  
├── pkg/tzkt/           # ✅ Completed: HTTP client for Tzkt API
├── scraper/            # Independent Go module
└── web/                # Independent Go module
```

### TDD Cycle
1. **RED**: Write failing test with clear error message
2. **GREEN**: Minimal implementation to pass test  
3. **REFACTOR**: Improve without changing behavior
4. **SKIP**: Use `t.Skip()` for clean commits during development

### Testing Strategy
- **Black box testing**: All tests use separate `_test` packages  
- **Unit tests**: Mock HTTP servers using `httptest.NewServer()` (fast, isolated)
- **Acceptance tests**: Real API calls, tagged `//go:build acceptance`
- **Parallel execution**: All tests use `t.Parallel()` for speed

## API Integration Details

### Tzkt API Mapping
| Field | Tzkt Response | Our Domain | Type |
|-------|---------------|------------|------|
| Delegator | `sender.address` | `delegator` | string |
| Amount | `amount` | `amount` | string |
| Block | `level` | `level` | string |  
| Time | `timestamp` | `timestamp` | string |

**Endpoint**: `GET https://api.tzkt.io/v1/operations/delegations`

## Key Decisions

- **Go 1.24 tool management**: No global installations, everything in `go.mod`
- **Multi-module workspace**: True service boundaries with independent versioning
- **Makefile with error handling**: Stops on first failure, clear progress messages
- **ATDD approach**: External API tests separate from unit tests
- **Minimal abstractions**: Standard library over frameworks

