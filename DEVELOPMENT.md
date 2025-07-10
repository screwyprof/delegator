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

### 🔄 In Progress: Tzkt API Client

**Current TDD Cycle**: RED → GREEN (ready for implementation)

**Specific Test**: `TestTzktClientGetDelegations` in `pkg/tzkt/client_acceptance_test.go`

**Test Intent**: Verify that our client can make a real HTTP call to Tzkt API and receive actual delegation data

**Current Status**: 
- Test properly fails with meaningful error: "Expected to receive delegations from Tzkt API, but got empty slice"
- Test currently skipped via `t.Skip()` to keep build green during infrastructure work
- `GetDelegations()` method returns `nil` (causes the failure)

**Implementation Decision**: 
- Use real HTTP calls in acceptance tests (not mocked)
- Test against live Tzkt API with small limit (10 delegations)
- Fail fast with clear error messages

**Next Step**: Implement HTTP call in `pkg/tzkt/client.go` 
- Remove `t.Skip()` from test
- Make `GetDelegations()` return actual data from `https://api.tzkt.io/v1/operations/delegations`

**Run Test**:
```bash
make test-acceptance-pkg  # Currently skipped - will fail when t.Skip() removed
```

## Planned Tasks

### Phase 1: Complete Tzkt API Client
- [ ] Implement HTTP call (GREEN phase)
- [ ] Refactor and improve error handling (REFACTOR phase)
- [ ] Add more test cases and edge cases
- [ ] Handle rate limits and retries

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
├── pkg/tzkt/           # 🔄 Current work: API client
├── scraper/            # Independent Go module
└── web/                # Independent Go module
```

### TDD Cycle
1. **RED**: Write failing test with clear error message
2. **GREEN**: Minimal implementation to pass test  
3. **REFACTOR**: Improve without changing behavior
4. **SKIP**: Use `t.Skip()` for clean commits during development

### Testing Strategy
- **Unit tests**: Fast, isolated, no external dependencies
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

