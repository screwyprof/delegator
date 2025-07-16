# Tezos Delegation Service

> **TL;DR**
> • Scraper → Postgres ← Web API  
> • `make run` starts the whole stack  
> • 80 % test coverage, lint & fmt clean  
> • Designed for Tzkt free-plan limits (10 req/s, 500 k req/day) - production would need additional throttling  
> • **Key insight**: Tzkt API rich filtering (`id.gt`) eliminates pagination complexity, unified ID-based approach simplifies architecture

The full exercise brief is in [TASK.md](TASK.md).  
In short, this implementation will:

1. Pull delegation operations from the public Tzkt API  
2. Persist them in PostgreSQL  
3. Serve them via `GET /xtz/delegations`, supporting pagination and an optional `year` filter

The demo is designed for the free-plan limits of Tzkt (10 req/s, 500 k req/day) and focuses on clean architecture and test-driven development. Live polling naturally respects these limits, though backfill would need additional throttling for production use. Future enhancements such as resilience or observability described in "Production Evolution".

---

## Goals
* Deliver an end-to-end delegation flow demo with two Go services and PostgreSQL.
* Design for the free Tzkt API tier (≤10 rps, 500 k requests/day) - live polling complies, backfill documents limitation.
* Maintain simple, readable code with ≥80 % test coverage and passing lint/format gates.
* Showcase a clear CQRS split: Scraper (write) and Web API (read).

## Non-Goals
* Importing the entire historical delegation dataset (controlled via checkpoint system).
* High-availability or multi-region deployment concerns.
* Advanced security hardening (auth, TLS termination, WAF).
* Additional API endpoints—only `GET /xtz/delegations` is implemented.
* Event-driven pipeline components (Normalizer, sharding, etc.)—outlined only in "Production Evolution".

---

## Quick start
```bash
# Quick start
$ make run               # start scraper, web, postgres
$ curl localhost:8080/xtz/delegations?page=1
```

## 1 Architecture Overview

```
Scraper → PostgreSQL ← Web API
```

* **CQRS split** – Polling (write-heavy) and querying (read-heavy) concerns are isolated.
* **Single database for the demo** – Simplifies local setup; can evolve to separate stores or an event-driven pipeline.
* **Go workspace** – Independent `scraper/`, `web/`, and `pkg/` modules allow clean boundaries while remaining in one repository.

---

## 2 System Architecture

```
Scraper → PostgreSQL ← Web API
```

* **Write path** – Scraper polls Tzkt API and persists delegation data
* **Read path** – Web API serves delegation data with pagination and filtering  
* **Storage** – PostgreSQL with optimized bulk inserts and automatic migrations
* **Observability** – Event-driven logging for operational visibility
* **Configuration** – Environment-based setup for demo, test, and production scenarios

## 3 Project Structure

Go workspace with independent modules:

```
delegator/
├── cmd/                       # Service entry points
│   ├── scraper/               # Scraper service
│   └── web/                   # Web API service
├── scraper/                   # Write-side module
├── web/                       # Read-side module  
├── pkg/                       # Shared utilities
├── migrations/                # Database schema
└── docker-compose.yaml        # Local development stack
```

---

## 4 Build Scope

The following items are implemented for the demo. They deliver a working slice of the overall delegation service while staying within the public `Tzkt API` limits (10 req/s, 500 k req/day). Extended capabilities like full historical back-fill, advanced resilience, observability are outlined later in Production Evolution.

### General Environment
* **Local run** – `make run` starts Docker Compose with both services and PostgreSQL.
* **Developer checks** – `make fmt`, `make lint`, and `make test` (race detector, verbose) must pass before commit. Use `make coverage` for detailed coverage reports with HTML output.
* **Shared env** – `.env` file must at least define `DB_DSN` used by both services.


### Scraper – write side
- Poll `https://api.tzkt.io/#operation/Operations_GetDelegations`.
- Extract and persist for each delegation:
  * `timestamp` (ISO 8601)
  * `amount` (string) – Tzkt returns an integer number of **mutez** (1 XTZ = 1 000 000 mutez). We expose that integer as a string—exactly like the task example (no decimal point).
  * `delegator` (sender.address)
  * `level` (block.height)
* **Event streaming**: Service emits 7 lifecycle events for complete observability
  * `BackfillStarted` / `BackfillDone` / `BackfillError` – backfill phase
  * `PollingStarted` / `PollingCycle` / `PollingShutdown` / `PollingError` – polling phase
  * **Enhanced shutdown tracking**: `PollingShutdown.Reason` captures context error for operational intelligence (graceful vs timeout scenarios)
* **Startup back-fill**: fetch delegations starting from stored checkpoint (0 = all data, higher ID = demo subset).
* **Chunked fetching**: page in chunks of `DELEGATIONS_PER_CHUNK` rows (default **500** for demo, **10,000** for production).
* **Continuous polling**: catch-up loop every `SCRAPER_INTERVAL` (default **10s**).
* **Smart filtering**: tzkt API supports rich filtering (`id.gt`, `timestamp.ge`, `id.le`, etc.) enabling gap-free pagination and duplicate prevention
* **Pure business logic**: Service has no logging dependencies; all observability via event streaming
* **Rate-limit awareness**: Live polling naturally respects limits (10s intervals). Backfill has no throttling for demo simplicity but documents the limitation for production use.
* **Payload optimization**: always append `select=id,timestamp,amount,sender.address,level`
* **Checkpointing**: maintains `LAST_PROCESSED_ID` for resumability via `Store.LastProcessedID()`
* **Atomic persistence**: `SaveBatch()` operations provide transaction-like guarantees and update checkpoint
* **Duplicate prevention**: database `UNIQUE(id)` constraint handles edge cases.
* **Pagination strategy**: unified `id.gt=lastID` for both backfill and live polling.
* **Clean shutdown**: `Start()` returns events channel and done channel for proper lifecycle management
* **Production validation**: successfully processes real Tezos delegation data (~5-6 delegations per hour, 132 in 24h is normal rate).

### Web API – read side
1. Endpoint `GET /xtz/delegations`.
2. Query parameters:
   * `year` (optional, YYYY)
   * Pagination: 50 items per page, newest-first order
3. Example response:
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
4. Return HTTP 400 for invalid input and HTTP 5xx on internal errors.
5. Error responses: always JSON `{ "code": 500, "error": "message" }` with appropriate HTTP status.
6. Service listens on `HTTP_PORT` (default **8080**) and exposes only this single path.

## 5 Data Model & Schema

### 5.1 Entity (Go)
```go
// Delegation represents a single delegation operation returned by Tzkt
// NOTE: field names match the JSON returned by our Web API, not Tzkt.
type Delegation struct {
    Timestamp string `json:"timestamp"` // ISO-8601 UTC
    Amount    string `json:"amount"`    // mutez string (exact integer)
    Delegator string `json:"delegator"` // sender.address from Tzkt
    Level     string `json:"level"`     // block.height
}
```

Field mapping (Tzkt → internal):
* `timestamp` → `Timestamp`
* `amount` (mutez) → `Amount` (string)
* `sender.address` → `Delegator`
* `level` → `Level`

### 5.2 Database Schema
```sql
-- Delegations table with UNIQUE constraint for duplicate prevention
CREATE TABLE delegations (
  id BIGINT PRIMARY KEY,           -- Tzkt operation ID (unique, strictly increasing)
  timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
  amount BIGINT NOT NULL,          -- Amount in mutez 
  delegator TEXT NOT NULL,         -- sender.address from Tzkt
  level BIGINT NOT NULL,           -- Block level/height
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Query optimization indexes
CREATE INDEX idx_delegations_timestamp ON delegations (timestamp);

-- Scraper checkpoint (singleton table)
CREATE TABLE scraper_checkpoint (
  single_row BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (single_row = TRUE),
  last_id BIGINT NOT NULL
);
```
---

## 6 Testing Strategy
* **Acceptance tests** – test scraper service against real Tzkt API and PostgreSQL; executed via `make test`.
* **Unit tests** – cover scraper service: polling/retry logic, store operations, and domain conversions.
* **Execution** – all test run with race detector, verbose output, and parallel execution.
* **Coverage** – aim ≥80 % on non-generated code; reported in test output.
* **TDD discipline** – RED → GREEN → REFACTOR cycle; acceptance first, then unit tests.

Quality tools (`make fmt`, `make lint`, `make test`, `make coverage`) run across the entire workspace, ensuring consistent standards across all packages.

> Coding conventions: idiomatic Go, gofumpt formatting, gci-ordered imports, explicit error wrapping.

---

## 7 Production Evolution

### 7.1 Current Production Status
* **Data collection**: Scraper successfully fetches and processes real Tezos delegation data from Tzkt API.
* **API integration**: Handles both small uncompressed (≤5 records) and large `gzip` compressed responses.
* **Architecture maturity**: Clean separation between public API contracts and implementation, with reusable utilities in `pkg/` packages.
* **Event-driven observability**: Complete lifecycle visibility through 7 event types, including enhanced shutdown reason tracking.
* **Single-threaded processing**: Current implementation processes delegations sequentially. While worker pools could parallelize fetching for production workloads, the current state processes complete historical data (760k+ delegations) in seconds, making additional complexity unjustified for demo purposes.

### 7.2 Data-Processing Pipeline
```
Scraper → Raw DB → Normalizer → Normalized DB → Web API
                         ↓
                     Event Queue
```
* **Scraper** continues to write raw JSON into an append-only table.  
* **Normalizer** (new component) transforms raw rows to a query-optimised schema, enriching and back-filling missing fields.  
* Web API switches to the Normalized DB—isolating read performance from write spikes.

### 7.3 Historical Backfill
* **Worker pool architecture**: Multi-worker framework processing time-based ranges in parallel for large-scale production environments.  
* **Checkpoints per worker**: Stored per worker to resume exactly where stopped.
* **Current efficiency baseline**: Single-threaded approach already processes 760k+ delegations in seconds, providing performance reference for complexity trade-offs.

### 7.4 Performance & Scalability
* **Indexes** on `timestamp`, `delegator`, `level`; partition tables by year.  
* **Connection pooling** via PgBouncer or built-in pool tuning.  
* **Worker pools**: Parallel processing becomes beneficial for sustained high-throughput scenarios or when API latency increases significantly beyond current ~0.06 seconds for full dataset.
* **Horizontal scale**: multiple scraper instances with modulo-based shard on `id`.

### 7.5 Resilience & Reliability
* **Rate limiting**: Token bucket limiter or fixed delays to respect API limits (10 req/sec for Tzkt).
* **Exponential back-off + circuit breaker** around Tzkt requests.  
* **HTTP 429 handling**: Respect `Retry-After` headers and implement adaptive throttling.
* **Health probes** (`/health/live`, `/health/ready`) for orchestrators.  
* **Graceful shutdown** drains in-flight HTTP requests and database txns.

### 7.6 Observability
* **Prometheus metrics** (`/metrics`) and **OpenTelemetry traces**.  
* Structured JSON logs with correlation IDs.

### 7.7 Security
* HTTPS termination, IP rate limiting, optional API keys.  
* Parameter sanitisation and prepared statements to prevent SQL injection.

### 7.8 Operations & CI/CD
* Docker images published per service; Helm charts for K8s.  
* GitHub Actions pipeline: fmt → lint → test → build → scan → deploy.  
* Zero-downtime migrations

### 7.9 API Extensions
* OpenAPI spec generation for clients and backends.

---
