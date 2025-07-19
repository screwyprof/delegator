# Tezos Delegation Service

> **TL;DR**
> • Scraper → Postgres ← Web API  
> • `make run` starts the whole stack  
> • 80% test coverage, lint & fmt clean  
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
├── migrator/                  # Database migration module
│   └── migrations/            # SQL migration files
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
  * `PollingStarted` / `PollingSyncCompleted` / `PollingShutdown` / `PollingError` – polling phase
  * **Enhanced shutdown tracking**: `PollingShutdown.Reason` captures context error for operational intelligence (graceful vs timeout scenarios)
* **Startup back-fill**: fetch delegations starting from stored checkpoint (0 = all data, higher ID = demo subset).
* **Chunked fetching**: page in chunks of `SCRAPER_CHUNK_SIZE` rows (default **10,000**).
* **Continuous polling**: catch-up loop every `SCRAPER_POLL_INTERVAL` (default **10s**).
* **Smart filtering**: tzkt API supports rich filtering (`id.gt`, `timestamp.ge`, `id.le`, etc.) enabling gap-free pagination and duplicate prevention
* **Pure business logic**: Service has no logging dependencies; all observability via event streaming
* **Rate-limit awareness**: Live polling naturally respects limits (10s intervals). Backfill has no throttling for demo simplicity but documents the limitation for production use.
* **Payload optimization**: always append `select=id,timestamp,amount,sender.address,level`
* **Checkpointing**: maintains checkpoint in `scraper_checkpoint` table (`last_id` column) for resumability via `Store.LastProcessedID()`
* **Atomic persistence**: `SaveBatch()` operations provide transaction-like guarantees and update checkpoint
* **Duplicate prevention**: database `UNIQUE(id)` constraint handles edge cases.
* **Pagination strategy**: unified `id.gt=lastID` for both backfill and live polling.
* **Clean shutdown**: `Start()` returns events channel and done channel for proper lifecycle management
* **Production validation**: successfully processes real Tezos delegation data (~5-6 delegations per hour, 132 in 24h is normal rate).

### Web API – read side
1. Endpoint `GET /xtz/delegations`.
2. Query parameters:
   * `year` (optional, YYYY format)
   * `page` (optional, default: 1) - Page number for pagination
   * `per_page` (optional, default: 50, max: 100) - Items per page
3. Pagination approach: **GitHub-style** with Link headers for navigation
   * API uses user-friendly `page`/`per_page` parameters
   * Internally converts to `LIMIT`/`OFFSET` for database queries
   * Response includes HTTP Link headers with rel="next", rel="prev", etc.
4. Example usage:
   ```
   GET /xtz/delegations?page=2&per_page=50&year=2023
   ```
5. Example response:
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
6. HTTP Link header for navigation:
   ```
   Link: <http://api.example.com/xtz/delegations?page=3&per_page=50&year=2023>; rel="next"
   ```
7. Return HTTP 400 for invalid input and HTTP 5xx on internal errors.
8. Error responses: always JSON `{ "code": 400, "message": "invalid year parameter: abc" }` with appropriate HTTP status.
9. Service listens on `WEB_HTTP_PORT` (default **8080**) and exposes only this single path.

### Pagination Architecture & Performance

#### Design Decision: GitHub-style Pagination with Performance Optimization
**Chosen approach:** `page`/`per_page` parameters with simplified Link headers
- **User Experience:** Intuitive page numbers, bookmarkable URLs  
- **API Surface:** `GET /xtz/delegations?page=2&per_page=50&year=2023`
- **Navigation:** HTTP Link headers with rel="next" and rel="prev" only (simplified approach)

**Performance optimization:** LIMIT n+1 technique instead of COUNT queries
- **Implementation:** Request pageSize+1 records to detect "has more pages" 
- **Benefit:** ~35x faster queries (0.5ms vs 17ms) by avoiding full table scans
- **Trade-off:** No "first" or "last" links, keeping only essential navigation

**Alternative considered:** Full RFC 5988 compatibility with all 4 rel types
- **Implementation:** Add rel="first" (always page=1) and rel="last" (requires COUNT)
- **Cost:** rel="first" is redundant, rel="last" requires expensive COUNT queries (35x slower)
- **Decision:** Prioritize performance and simplicity over complete standards compliance

**Alternative considered:** GitHub's hybrid pagination approach  
- **GitHub's method:** Accept `page` parameters, return `before`/`after` cursor tokens in Link headers
- **Benefits:** Combines page UX with cursor performance, handles data consistency during navigation
- **Decision:** Too complex for demo scope - our simple approach provides clear, predictable behavior

**Rationale:** Simple approach provides essential navigation with optimal performance. Users can always construct "first" page (page=1) themselves, and "last" page functionality isn't worth 35x performance penalty.

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
-- Create delegations table
CREATE TABLE IF NOT EXISTS delegations (
    id BIGINT PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    amount BIGINT NOT NULL,
    delegator TEXT NOT NULL,
    level BIGINT NOT NULL,
    year INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create standalone timestamp index for default queries without year filtering
CREATE INDEX IF NOT EXISTS idx_delegations_timestamp ON delegations (timestamp DESC); 

-- Create composite index for optimal year filtering and pagination
CREATE INDEX IF NOT EXISTS idx_delegations_year_timestamp ON delegations (year, timestamp DESC); 

-- Scraper checkpoint (singleton table)
CREATE TABLE scraper_checkpoint (
  single_row BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (single_row = TRUE),
  last_id BIGINT NOT NULL
);
```

### 5.3 Performance Optimization

**Dual-index strategy for optimal query performance:**

1. **Year column**: `year INTEGER` populated from timestamp for direct filtering
2. **Composite index**: `(year, timestamp DESC)` for year-filtered pagination  
3. **Timestamp index**: `(timestamp DESC)` for default pagination

**Performance benefit**: Direct column filtering (`year = 2025`) vs function calls (`EXTRACT(YEAR FROM timestamp) = 2025`) eliminates full table scans.

### 5.4 Environment Variables

We use [caarlos0/env](https://github.com/caarlos0/env) for environment variable parsing with validation and better error messages.

#### Scraper Service
| Variable | Default | Description |
|----------|---------|-------------|
| `SCRAPER_DATABASE_URL` | `postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable` | PostgreSQL connection string |
| `SCRAPER_CHUNK_SIZE` | `10000` | Delegations per API request |
| `SCRAPER_POLL_INTERVAL` | `10s` | Interval between polling cycles |
| `SCRAPER_INITIAL_CHECKPOINT` | `0` | Starting checkpoint (0 = full sync) |
| `SCRAPER_HTTP_CLIENT_TIMEOUT` | `30s` | HTTP client timeout |
| `SCRAPER_TZKT_API_URL` | `https://api.tzkt.io` | TzKT API base URL |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_HUMAN_FRIENDLY` | `false` | Human-readable log format |

#### Web API Service
| Variable | Default | Description |
|----------|---------|-------------|
| `WEB_DATABASE_URL` | `postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable` | PostgreSQL connection string |
| `WEB_HTTP_HOST` | `localhost` | Server bind address |
| `WEB_HTTP_PORT` | `8080` | Server port |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_HUMAN_FRIENDLY` | `false` | Human-readable log format |

#### Test Configuration

Acceptance tests are configurable via environment variables using independent `testcfg` packages per module:

**Scraper Tests** (`scraper/testcfg`):
| Variable | Default | Description |
|----------|---------|-------------|
| `SCRAPER_TEST_CHUNK_SIZE` | `1000` | Test batch size |
| `SCRAPER_TEST_CHECKPOINT` | `1939557726552064` | Test starting checkpoint |
| `SCRAPER_TEST_POLL_INTERVAL` | `100ms` | Test polling interval |
| `SCRAPER_TEST_SHUTDOWN_TIMEOUT` | `2s` | Test shutdown timeout |

**Tzkt Client Tests** (`pkg/tzkt/testcfg`):
| Variable | Default | Description |
|----------|---------|-------------|
| `TZKT_TEST_LIMIT` | `5` | API request limit for tests |
| `TZKT_TEST_OFFSET` | `100000` | API request offset for stable test data |
| `TZKT_TEST_HTTP_TIMEOUT` | `30s` | HTTP client timeout |
| `TZKT_TEST_BASE_URL` | `https://api.tzkt.io` | TzKT API base URL |

**Web API Tests** (`web/testcfg`):
| Variable | Default | Description |
|----------|---------|-------------|
| `WEB_TEST_SEED_CHECKPOINT` | `1939557726552064` | Database seeding checkpoint |
| `WEB_TEST_SEED_CHUNK_SIZE` | `1000` | Database seeding batch size |
| `WEB_TEST_SEED_TIMEOUT` | `5s` | Database seeding timeout |
| `WEB_TEST_LOG_LEVEL` | `info` | Test logging level for SUT observability |
| `WEB_TEST_LOG_HUMAN_FRIENDLY` | `true` | Human-readable test log format |

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
