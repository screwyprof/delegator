# Tezos Delegation Service

> **TL;DR**
> • Scraper → Postgres ← Web API  
> • `make run` starts the whole stack  
> • 80 % test coverage, lint & fmt clean  
> • Fits Tzkt free-plan limits (10 req/s, 500 k req/day)


The full exercise brief is in [TASK.md](TASK.md).  
In short, this implementation will:

1. Pull delegation operations from the public Tzkt API  
2. Persist them in PostgreSQL  
3. Serve them via `GET /xtz/delegations`, supporting pagination and an optional `year` filter

The demo stays within the free-plan limits of Tzkt (10 req/s, 500 k req/day) and focuses on clean architecture and test-driven development. Future enhancements—full historical back-fill, resilience, observability—are described in “Production Evolution”.

---

## Goals
* Deliver an end-to-end delegation flow demo with two Go services and PostgreSQL.
* Stay inside the free Tzkt API tier (≤10 rps, 500 k requests/day).
* Maintain simple, readable code with ≥80 % test coverage and passing lint/format gates.
* Showcase a clear CQRS split: Scraper (write) and Web API (read).

## Non-Goals
* Importing the entire historical delegation dataset (only last 1 000 delegations or ≤14 days).
* High-availability or multi-region deployment concerns.
* Advanced security hardening (auth, TLS termination, WAF).
* Additional API endpoints—only `GET /xtz/delegations` is implemented.
* Event-driven pipeline components (Normalizer, sharding, etc.)—outlined only in “Production Evolution”.

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

## 2 Project Structure (Demo Layout)
Uber-style monorepo with independent Go modules with clear boundaries.

```
delegator/                     # Go workspace with independend modules
├── Makefile                   # Build and development tasks
├── cmd/
│   ├── scraper/main.go        # Scraper service entry point
│   └── web/main.go            # Web API entry point
├── scraper/                   # Write side (independent Go module)
│   ├── poller/                # Polling loop that uses pkg/tzkt client
│   ├── delegation/            # Domain models and handlers
│   └── store/                 # Data persistence layer
├── web/                       # Read side (independent Go module)
│   ├── handler/               # HTTP handlers & routing
│   ├── store/                 # Query logic
├── pkg/                       # Shared libraries (independent Go module)
│   ├── tzkt/                  # Reusable Tzkt HTTP client
└── docker-compose.yaml        # Local dev stack
```

---

## 3 Build Scope

The following items are implemented for the demo. They deliver a working slice of the overall delegation service while staying within the public `Tzkt API` limits (10 req/s, 500 k req/day). Extended capabilities like full historical back-fill, advanced resilience, observability are outlined later in Production Evolution.

### General Environment
* **Local run** – `make run` starts Docker Compose with both services and PostgreSQL.
* **Developer checks** – `make fmt`, `make lint`, and `make test` (race detector, verbose, coverage report) must pass before commit.
* **Shared env** – `.env` file must at least define `DB_DSN` used by both services.


### Scraper – write side
- Poll `https://api.tzkt.io/#operation/Operations_GetDelegations`.
- Extract and persist for each delegation:
  * `timestamp` (ISO 8601)
  * `amount` (string) – Tzkt returns an integer number of **mutez** (1 XTZ = 1 000 000 mutez). We expose that integer as a string—exactly like the task example (no decimal point).
  * `delegator` (sender.address)
  * `level` (block.height)
* Startup back-fill: fetch last 1 000 delegations **or** ≤ 14 days using `timestamp.ge=<now-LOOKBACK_HOURS>`.
* Chunked fetching: page in chunks of `DELEGATIONS_PER_CHUNK` rows (default **500**).
* Continuous catch-up loop every `SCRAPER_INTERVAL` (default **10s**).
* Initial query window set by `LOOKBACK_HOURS` (default **336** ≈ 14 days) using `timestamp.ge` filter.
* Subsequent cycles use `id.gt=LAST_PROCESSED_ID` to fetch only new delegations.
* Network robustness: simple retry with fixed back-off (max 3 attempts) on transport errors or 5xx.
* Rate-limit awareness – scraper enforces `SCRAPER_RPS_LIMIT` (≤10) and honours `Retry-After`; falls back to 1 rps when soft daily cap reached.
* Payload trimming – always append `select=id,timestamp,amount,sender.address,level` and send `Accept-Encoding: gzip`.
* Checkpointing – maintains `LAST_PROCESSED_ID` in `scraper_checkpoint` table for resumability.
* Operate continuously; duplicate prevention via composite primary key `(level, delegator)`.

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

## 4 Data Model & Schema

### 4.1 Entity (Go)
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

### 4.2 Database Schema (Demo)
```sql
CREATE TABLE delegations (
  id BIGINT PRIMARY KEY,
  timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
  amount BIGINT NOT NULL,
  delegator TEXT NOT NULL,
  level BIGINT NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_delegations_year ON delegations ((date_part('year', timestamp)));

-- Scraper checkpoint
CREATE TABLE scraper_checkpoint (
  last_id BIGINT PRIMARY KEY
);
```
---

## 5 Testing Strategy
* **Acceptance test** – starts the Web API (with test data) and verifies the HTTP contract; executed via `make test-acceptance`.
* **Unit tests** – cover both services: scraper polling/retry logic, DAO functions, and Web API handlers/validators.
* **Execution** – `make test` runs all modules’ tests (`./...`) with race detector, verbose output, and parallel tests.
* **Coverage** – aim ≥80 % on non-generated code; reported in test output.
* **TDD discipline** – RED → GREEN → REFACTOR cycle; acceptance first, then unit tests.

Quality tools (`make fmt`, `make lint`, `make test`) run across the entire workspace, ensuring both services meet the same standards.

> Coding conventions: idiomatic Go, gofumpt formatting, gci-ordered imports, explicit error wrapping.

---

## 6 Production Evolution

### 6.1 Data-Processing Pipeline
```
Scraper → Raw DB → Normalizer → Normalized DB → Web API
                         ↓
                     Event Queue
```
* **Scraper** continues to write raw JSON into an append-only table.  
* **Normalizer** (new component) transforms raw rows to a query-optimised schema, enriching and back-filling missing fields.  
* Web API switches to the Normalized DB—isolating read performance from write spikes.

### 6.2 Historical Backfill
* Multi-worker framework processing time-based ranges in parallel.  
* Checkpoints stored per worker to resume exactly where stopped.

### 6.3 Performance & Scalability
* **Indexes** on `timestamp`, `delegator`, `level`; partition tables by year.  
* **Connection pooling** via PgBouncer or built-in pool tuning.  
* **Horizontal scale**: multiple scraper instances with modulo-based shard on `id`.

### 6.4 Resilience & Reliability
* **Exponential back-off + circuit breaker** around Tzkt requests.  
* **Health probes** (`/health/live`, `/health/ready`) for orchestrators.  
* **Graceful shutdown** drains in-flight HTTP requests and database txns.

### 6.5 Observability
* **Prometheus metrics** (`/metrics`) and **OpenTelemetry traces**.  
* Structured JSON logs with correlation IDs.

### 6.6 Security
* HTTPS termination, IP rate limiting, optional API keys.  
* Parameter sanitisation and prepared statements to prevent SQL injection.

### 6.7 Operations & CI/CD
* Docker images published per service; Helm charts for K8s.  
* GitHub Actions pipeline: fmt → lint → test → build → scan → deploy.  
* Zero-downtime migrations

### 6.8 API Extensions
* OpenAPI spec generation for clients and backends.

---
