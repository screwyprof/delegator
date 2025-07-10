# Delegator - Tezos Delegation Service

A Go-based service that collects Tezos delegation data and serves it through a public API.

## Project Goal

Build a system that:
- Collects delegation data from the Tzkt API
- Stores it in a database
- Serves it through a REST API with pagination and filtering
- Handles the challenge of processing years of historical data

## Architecture

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

**Data Source:** [Tzkt API Delegations Endpoint](https://api.tzkt.io/#operation/Operations_GetDelegations)

## Getting Started

```bash
# Install development tools
make deps

# See all available commands
make help

# Run the services
make run-scraper   # Start scraper service
make run-web       # Start web API service
```

## Requirements

- **Go 1.24+** 
- Docker & Docker Compose
- PostgreSQL

## Development

For development workflow, current status, and implementation details:
**[📋 DEVELOPMENT.md](DEVELOPMENT.md)** - Development guide and task tracking

---

This project demonstrates building a data-intensive service with clean architecture that can evolve from prototype to production scale. 