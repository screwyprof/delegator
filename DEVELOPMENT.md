# Development Guide

## Quick Start

### Environment Setup
**Nix users**: `direnv allow` (provides Go 1.24 environment automatically)

```bash
# Setup development environment
make deps              # Install Go 1.24 tools
make help              # See all available commands

# Development workflow  
make                   # Complete development workflow (default)
make check             # format → lint → test
make fmt               # Format code + organize imports  
make lint              # Run static analysis
make test              # Unit tests with race detection

# Testing
make coverage          # Generate coverage report (HTML + console)
make test-acceptance   # Run acceptance tests

# Running services
make run-scraper       # Start scraper service
make run-web           # Start web API service
```

## Key Decisions

- **Go 1.24 tool management**: No global installations, everything in `go.mod`
- **Multi-module workspace**: True service boundaries with independent versioning
- **Self-documenting Makefile**: Automates workspace development across all modules
- **ATDD approach**: External API tests separate from unit tests
- **Minimal abstractions**: Standard library over frameworks
- **Pure dependency injection**: Explicit dependencies over hidden configuration
- **Descriptive test helpers**: DRY compliance with self-documenting function names

## Development Log & Current Status

### ✅ Completed Infrastructure
- [x] Go workspace with independent service modules
- [x] Build system with Go 1.24 tool management  
- [x] ATDD test infrastructure with build tags
- [x] Documentation and development guides

### ✅ Completed: Tzkt API Client (Optimized for Cost Efficiency)

**What I Built**: Implements an optimized HTTP client for the Tzkt API with a focus on staying within free-plan limits:
```go
GetDelegations(ctx context.Context, req DelegationsRequest) ([]Delegation, error)
```

**Key Optimizations**:
- **GZIP Compression**: Automatic `Accept-Encoding: gzip` header reduces bandwidth usage
- **Field Selection**: Uses `select=id,timestamp,amount,sender,level` to fetch only necessary fields
- **67% bandwidth reduction**: From 889 bytes to 293 bytes per 2-delegation response


**API Efficiency**: 
- Stays within Tzkt free-plan limits (≤10 rps, 500k req/day)
- Supports up to 5,000 delegations/second (10 rps × 500 delegations/request)
- Minimized payload trimming reduces costs and improves performance

**Key Decision**: Client accepts pre-configured HTTP client for production use

**Production Considerations**: For continuous polling, would need retry logic with exponential backoff, circuit breaker pattern, rate limiting, and response body size limits - could be part of the client itself or a higher-level component using the client. For now, keeping it simple.

**Status**: Working, tested, optimized, ready for scraper integration

## Planned Tasks

### Scraper Service
- [ ] Tzkt API polling logic
- [ ] Historical data backfill
- [ ] Checkpointing system with ID-based pagination
- [ ] Retry logic with exponential backoff

### Web API Service
- [ ] HTTP handlers for `/xtz/delegations` endpoint
- [ ] Year filtering and pagination
- [ ] JSON response formatting
- [ ] Error handling and validation

### Infrastructure
- [ ] Database schema and migrations
- [ ] Docker Compose setup
- [ ] Database integration
- [ ] Basic logging and error handling

## Development Tools & Workflow

### Available Tools
| Tool | Purpose | Command |
|------|---------|---------|
| `gci` | Import organization | `make fmt` |
| `gofumpt` | Code formatting (stricter than gofmt) | `make fmt` |
| `golangci-lint` | Static analysis (50+ linters) | `make lint` |

### Project Architecture
```
delegator/              # Go workspace root
├── cmd/                # Service entry points
│   ├── scraper/        # Write side (CQRS)
│   └── web/            # Read side (CQRS)  
├── pkg/tzkt/           # ✅ Complete: Optimized HTTP client for Tzkt API
├── scraper/            # Independent Go module
└── web/                # Independent Go module
```

### Testing Strategy
- **ATDD approach**: Acceptance tests drive development
- **Black box testing**: All tests use separate `_test` packages  
- **Acceptance tests**: Real API calls, tagged `//go:build acceptance`
- **Parallel execution**: All tests use `t.Parallel()` for speed

## API Integration Details

### Tzkt API Mapping
| Field | Tzkt Response | Our Domain | Type | Notes |
|-------|---------------|------------|------|-------|
| ID | `id` | `id` | int64 | For checkpointing and pagination |
| Delegator | `sender.address` | `delegator` | string | Tezos address |
| Amount | `amount` | `amount` | string | Mutez as string |
| Block | `level` | `level` | string | Block height |
| Time | `timestamp` | `timestamp` | string | ISO-8601 UTC |

**Optimized Endpoint**: `GET https://api.tzkt.io/v1/operations/delegations`
- **Query Parameters**: `limit`, `offset`, `select=id,timestamp,amount,sender,level`
- **Headers**: `Accept-Encoding: gzip`
- **Bandwidth Savings**: 67% reduction in payload size

### Performance Metrics
```bash
# Before optimization: 889 bytes for 2 delegations
curl "https://api.tzkt.io/v1/operations/delegations?limit=2&offset=0"

# After optimization: 293 bytes for 2 delegations  
curl -H "Accept-Encoding: gzip" \
     "https://api.tzkt.io/v1/operations/delegations?limit=2&offset=0&select=id,timestamp,amount,sender,level"
```