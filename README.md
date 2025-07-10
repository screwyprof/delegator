# Delegator - Tezos Delegation Service

A Go-based service that collects Tezos delegation data and serves it through a public API.

## Project Goal

Build a system that:
- Collects delegation data from the Tzkt API
- Stores it in a database
- Serves it through a REST API with pagination and filtering
- Handles the challenge of processing years of historical data

## Project Background

This implements the [Tezos delegation service exercise](TASK.md) as a demonstration of:
- Clean architecture with CQRS pattern
- Go service design  
- Database integration
- API development

**Time constraint**: 3-hour implementation focusing on working system over feature completeness.

## Architecture Plan

Two separate Go services using CQRS pattern:

```
┌─────────────────┐    ┌──────────────┐    ┌─────────────────┐
│   Scraper       │───▶│  PostgreSQL  │◀───│   Web API       │
│   (Write Side)  │    │              │    │   (Read Side)   │
│                 │    │ Shared Store │    │                 │
│ • Polls Tzkt    │    │              │    │ • Serves HTTP   │
│ • Checkpoints   │    │              │    │ • Pagination    │
│ • Retry logic   │    │              │    │ • Filtering     │
└─────────────────┘    └──────────────┘    └─────────────────┘
```

**Why separate services?**
- Independent scaling and deployment
- Clear separation of concerns
- Better fault isolation
- Easier to evolve to microservices

## API Specification

### GET /xtz/delegations

**Query Parameters:**
- `year` (optional): Filter by year (YYYY format)
- Pagination: 50 items per page, newest first

**Response Format:**
```json
{
  "data": [
    {
      "timestamp": "2022-05-05T06:29:14Z",
      "amount": "125896", 
      "delegator": "tz1a1SAaXRt9yoGMx29rh9FsBF4UzmvojdTL",
      "level": "2338084"
    }
  ]
}
```

## Data Model

**Delegation Entity:**
- `timestamp`: ISO 8601 format string
- `amount`: String representation of amount
- `delegator`: Tezos address string (from sender.address)
- `level`: String representation of block height

**Data Source:** [Tzkt API Delegations Endpoint](https://api.tzkt.io/#operation/Operations_GetDelegations)

## Project Structure

```
delegator/
├── go.work               # Go workspace configuration
├── Makefile              # Build and development tasks
├── cmd/
│   ├── scraper/          # Scraper service entry point
│   └── web/              # Web API service entry point
├── scraper/              # Write side (CQRS)
│   ├── poller/           # Tzkt API polling
│   ├── delegation/       # Domain models
│   └── store/            # Data persistence
├── web/                  # Read side (CQRS)
│   ├── handler/          # HTTP handlers
│   └── store/            # Query operations
├── pkg/                  # Shared utilities
└── migrations/           # Database migrations
```

## Implementation Requirements

### Scraper Service
- Poll Tzkt API for new delegations
- Handle historical data backfill
- Implement checkpointing system
- Retry logic with exponential backoff
- Graceful error handling

### Web API Service
- Single HTTP endpoint with proper error handling
- Year-based filtering with database indexes
- Pagination (50 items per page)
- Response sorting (newest first)

### Shared Infrastructure
- PostgreSQL database
- Docker Compose for local development
- Database migrations
- Basic logging and error handling

## Current Status

**Project foundation complete.** Basic service structure implemented with working entry points.

### Quick Start

```bash
# Show available commands
make help

# Run services
make run-scraper    # Start scraper service
make run-web        # Start web API service

# Development tasks
make fmt           # Format code
make vet           # Run go vet
make build         # Build both services
```

## Development Plan

1. ✅ **Setup** - Go workspace, service structure, build tooling
2. **Core Services** - Implement scraper and web API with basic functionality
3. **Testing** - Unit tests, integration tests, HTTP tests
4. **Documentation** - Setup instructions and API documentation

## Production Considerations

While this implementation focuses on demonstrating clean architecture patterns, a production system would need:

- **Monitoring**: Health checks, metrics, structured logging
- **Resilience**: Circuit breakers, advanced retry strategies
- **Security**: Authentication, input validation, rate limiting
- **Scale**: Caching, connection pooling, horizontal scaling
- **Operations**: Kubernetes deployment, CI/CD pipelines, alerting

## Development Requirements

- Go 1.24+
- Docker & Docker Compose
- PostgreSQL

*Note: Nix users can use the provided flake.nix for automated environment setup.*

---

This project demonstrates building a data-intensive service with clean architecture that can evolve from prototype to production scale. 