# Development Journey – Capturing the Why

This document is a living dev-diary recording **how** and **why** we built Delegator.  It preserves the experiments, dead-ends, and "aha!" moments that shaped the current code-base so reviewers can trace every decision back to evidence.

---

## 1 Environment & Tooling

- Go 1.24 (single version across the repo)
- Nix flakes + direnv: `direnv allow && make run`
- Single Go workspace: `scraper`, `web`, `pkg`, `migrator`
- Makefile: `make help`, `make check`, `make run`
- Configuration: `caarlos0/env` for environment variable parsing with validation

---

## 2 Tzkt API Client (`pkg/tzkt`)

### 2.1 Goals
* Provide a _single_ call:
  ```go
  GetDelegations(ctx, DelegationsRequest) ([]Delegation, error)
  ```
* Minimally couple callers – hide URL gymnastics & HTTP details.
* Optimise for the free-tier quota (10 req/s, 500k req/day).
* Fail predictably with sentinel errors.

### 2.2 Experiments & Findings
1. **Field selection**  
   `select=id,timestamp,amount,sender,level` shrinks payloads by ~67% compared to the full response—ample savings for the free-tier quota.
2. **Compression threshold**  
   Gzip kicks in around six rows; Go's `http.Client` decompresses transparently – no code required.
3. **Pagination flavours**  
   Offset, `timestamp.ge`, and `id.gt` filters were explored via the API docs and quick `curl` tests. `id.gt` proved stateless, deterministic, and suitable for both back-fill and live polling—making it the clear winner.

### 2.3 Implementation Highlights
* **Explicit error categories** – the client surfaces network, status-code, and decoding issues separately so the scraper can decide when to retry or abort.
* **Tests** – cover common failure modes and include an acceptance test against the real API.
* **Essential filters implemented** – request struct supports `limit`, `offset`, `id.gt`, and `timestamp.ge`, covering every pattern the scraper needs for back-fill and live polling.

_Status_: ✅ Stable. No TODOs unless the upstream API changes.

---

## 3 Scraper Service (`scraper/`)

### 3.1 Problem Statement
Continuously ingest _every_ delegation exactly once and stay current, all while honouring the public API limits.

### 3.2 File Organization
The scraper code is organized across three files within the same package:

| File | Purpose | Contents |
|------|---------|----------|
| **`scraper.go`** | **Contracts & Types** | Interfaces (`Client`, `Store`, `Clock`), Events, Errors, Constants |
| **`service.go`** | **Service Implementation** | Service methods, Options, Constructor, Business logic |
| **`subscriber.go`** | **Event Handling** | Event subscription and routing utilities |

This file organization makes the codebase easier to navigate.

### 3.3 Key Decisions
| Area | Choice | Why |
|------|--------|-----|
| **Pagination** | **Unified `id.gt` filter** (TzKT parameter "ID greater than") drives both back-fill and live polling | Avoids the previously-considered dual code paths (time-based back-fill with `timestamp.ge` + ID-based live polling), eliminating state-drift edge cases. |
| **Batch size** | **10,000** (configurable; tests use smaller values for speed) | Matches the default in code. Integration tests dial it down to keep CI snappy. |
| **Poll interval** | **10 s** | One call every 10 s (0.1 req/s) stays safely under the 10 req/s free-tier limit and still exceeds the 8 s Tezos block time–so we never miss a delegation. |
| **Options** | Functional options set chunk size & poll interval; clock injection keeps tests deterministic | Keeps constructor minimal |
| **Event streaming** | 7 event types covering complete scraper lifecycle | Eliminates callbacks and logger dependencies, enabling pure business logic with composable observability. |
| **Storage boundary** | `Store` interface with atomic `SaveBatch()` | Simplified from transaction-based approach; allows mock-first TDD with cleaner semantics. |

### 3.4 Control Flow
```text
Start() → events channel + done channel
   ↓
Backfill phase: syncBatch() until no more data
   ↓
Polling phase: syncBatch() every 10s until context cancelled
   ↓
Event emission: BackfillStarted, BackfillDone, PollingStarted, PollingCycle, etc.
```

The service now uses `Start()` which returns an events channel and done channel for clean shutdown:
- Events channel receives all lifecycle events (backfill, polling, errors)
- Done channel signals when service has completely shut down
- Context cancellation triggers graceful shutdown

### 3.5 Event Streaming
Seven event types provide complete observability:
* **BackfillStarted** / **BackfillDone** / **BackfillError** – backfill phase lifecycle
* **PollingStarted** / **PollingCycle** / **PollingShutdown** / **PollingError** – polling phase lifecycle

Benefits of this approach:
* **Pure business logic** – Service has no logging dependencies
* **Composable observability** – main.go handles all logging via event subscriptions
* **Better testability** – Events provide deterministic testing points

### 3.6 Failure Handling
* **Graceful shutdown via context** – every loop checks `ctx.Done()` and returns promptly
* **Throttling** – intentionally absent; 429s are virtually impossible at our current cadence, but a token-bucket limiter is easy to add if future requirements change.

### 3.7 Real-World Validation (2025-07-16)
* **Full history** (~761k rows) persisted in ~15s on a laptop; zero 429s.
* **End-to-end service throughput**: ~50,728 records/second (includes API calls, JSON parsing, domain conversion, and database operations)
* **API efficiency**: ~77 API calls (760,930 ÷ 10000 chunk size)
* **Live rate**: ~7.5 delegations/hour ⇒ our 10s ticker is wildly safe
* **No 429s observed** even during back-fill bursts
* **Event streaming** provides complete visibility into scraper operations
* **PostgreSQL integration** – migrations, connection pooling, and bulk inserts are already in place.

### 3.8 Bulk Inserts at Speed
Switched from pgx.Batch to pgx.CopyFrom (temp-table + `ON CONFLICT DO NOTHING`) – roughly 7× faster.

### 3.9 Key Take-Aways

- Unified `id.gt` pagination keeps back-fill and live polling on the same path.
- Event streaming decouples business logic from logging/observability.
- `CopyFrom` bulk insert gives **~7×** speed-up vs `pgx.Batch`.
- 10k-row chunks + 10s ticker stay well below free-tier limits.

### 3.10 Environment Variables

The scraper is configured entirely via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SCRAPER_DATABASE_URL` | `postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable` | PostgreSQL connection string |
| `SCRAPER_CHUNK_SIZE` | `10000` | Delegations per API request |
| `SCRAPER_POLL_INTERVAL` | `10s` | Interval between polling cycles |
| `SCRAPER_INITIAL_CHECKPOINT` | `0` | Starting checkpoint (0 = full sync) |
| `SCRAPER_HTTP_CLIENT_TIMEOUT` | `30s` | HTTP client timeout |
| `SCRAPER_TZKT_API_URL` | `https://api.tzkt.io` | TzKT API base URL |

Handy recipes:
```bash
make run-scraper       # full historical sync
make run-scraper-demo  # recent data only
```

_Status_: ✅  Ready for the demo; Web API still pending.

---

## 4 Web API Service (`web/`)

### 4.1 Problem Statement
Expose collected delegation data via HTTP endpoint `GET /xtz/delegations` with pagination and year filtering.

### 4.2 Key Decisions
| Area | Choice | Why |
|------|--------|-----|
| **Architecture** | **Clean separation**: `web/api/` (contracts), `web/handler/bind/` (HTTP parsing), `web/handler/` (orchestration) | Keeps API contracts pure; HTTP conversion logic separate from business logic |
| **Pagination** | **GitHub-style** (`page`/`per_page` + Link headers) | Demo-friendly API, familiar to developers |
| **Error handling** | **Sentinel errors** for validation failures | Better testing and error categorization |


### 4.3 Environment Variables

The web API is configured entirely via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `WEB_DATABASE_URL` | `postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable` | PostgreSQL connection string |
| `WEB_HTTP_HOST` | `localhost` | Server bind address |
| `WEB_HTTP_PORT` | `8080` | Server port |

Handy recipes:
```bash
make run-web           # start web API service
```

### 4.4 Key Takeaway: Pagination Strategy

**The Question**: How do you paginate through 760k+ records efficiently?

**The Candidates**:
- **GitHub-style**: `page=5&per_page=50` (familiar, bookmarkable)
- **Google-style**: `page_token=CjkKEw...` (performant, consistent)
- **RFC 5988**: All 4 rel types (`first`, `prev`, `next`, `last`)

**The Decision Process**:

1. **Started with user experience** - developers expect `page=5`, not cryptic tokens
2. **Discovered the performance trap** - `rel="last"` requires COUNT(*) queries (35x slower!)
3. **Found the sweet spot** - keep essential navigation (`prev`/`next`), drop the expensive bits

**What we built**: `GET /xtz/delegations?page=5&per_page=50` → gets you there in 0.8ms

**The Plot Twist**: Testing GitHub's actual API revealed they do hybrid pagination - accept friendly `page=5` requests but return optimized cursor tokens in Link headers. Engineering excellence hidden behind simple UX!

```bash
# GitHub's secret: page numbers IN, cursor tokens OUT
curl -I "https://api.github.com/repos/octocat/Spoon-Knife/issues?page=5&per_page=2"
# → Returns: ?page=6&per_page=2&after=Y3Vy... (cursor tokens for performance)
```

**Bottom line**: We chose honest simplicity over hidden complexity. Our OFFSET approach is transparent, predictable, and plenty fast for demo scale. Sometimes the best engineering decision is the one you can explain in 30 seconds. 🎯

### 4.5 Current Status
✅ PostgreSQL integration with pagination and year filtering  
✅ GitHub-style pagination with Link headers  
✅ Clean architecture with bind package for HTTP parsing  

### 4.6 Database Optimization Decision
**Problem**: Initial queries took 132ms (parallel seq scan + sort on 761k rows)
**Solution**: Year column + dual indexes for different access patterns

```sql
CREATE INDEX idx_delegations_timestamp ON delegations (timestamp DESC);           -- Default pagination  
CREATE INDEX idx_delegations_year_timestamp ON delegations (year, timestamp DESC); -- Year filtering
```

**Results**: Default pagination 0.074ms (1,792x faster), year filtering 0.331ms, deep pagination 0.858ms - validates GitHub-style approach.

**Pagination Implementation**: Uses LIMIT n+1 technique for efficiency - requests pageSize+1 records to detect "has more" without expensive COUNT queries. Simplified Link headers (rel="prev" and rel="next" only) match GitHub's actual API behavior, providing 35x performance gain by omitting COUNT-based "last" links.

_Status_: Database integration complete. API ready for demo.

---

## 5 Migrator – Fast Test Database Setup

### 5.1 Goals
- **Fast tests**: Use `pgtestdb` which utilises template databases for instant test setup
- **Two test types**: Some tests need empty schema, others need realistic data
- **Zero setup time**: Template databases eliminate migration overhead per test

### 5.2 Implementation

**Two migrator types for different test needs:**

```go
// Schema-only tests (scraper acceptance tests)
testDB := migratortest.CreateScraperTestDatabase(t, "migrations", checkpoint)

// Data-seeded tests (web API acceptance tests)  
testDB := migratortest.CreateSeededTestDatabase(t, "migrations", demoCheckpoint, chunkSize, timeout)
```

**Key insight**: `pgtestdb` creates template databases once, then clones them instantly for each test. No repeated migrations.

### 5.3 Why It Works
- **Template database pattern**: Schema + seed data prepared once, cloned per test
- **`SchemaMigrator`**: Just runs SQL migrations for clean schema tests
- **`SeededMigrator`**: Runs migrations + uses scraper to populate realistic delegation data
- **Fast parallel tests**: Each test gets isolated database in milliseconds

### 5.4 Key Takeaways
- Use `pgtestdb` for instant database cloning from templates
- Separate migrators for schema-only vs data-seeded test scenarios
- Seeded migrator actually runs the scraper to create realistic test data
- Template approach makes parallel tests feasible and fast

_Status_: ✅ Tests run in parallel with zero database setup overhead.

---

## 6 Test Configuration Strategy

### 6.1 Key Decision: Environment-Configurable Tests  
Created independent `testcfg` packages per module rather than hardcoded test constants.

**Why**: Acceptance tests need to adapt between local development (fast iteration), CI environments (conservative timeouts), and debugging scenarios (more data, slower execution). Hardcoded values meant tests either ran slowly everywhere or failed in slower environments.

### 6.2 Implementation Approach
Each module owns its test configuration semantics via dedicated `testcfg` packages. Used the same `caarlos0/env` pattern as production configurations for consistency.

**Key insight**: Service independence extends to test configuration—scraper tests care about chunk sizes and poll intervals, web tests care about seed data timeouts, tzkt client tests care about API limits.

_Status_: ✅ Tests adapt to any environment without code changes.

---

## 7 Database Configuration Strategy

Dev: tmpfs Postgres for quick iterations.  
Prod: standard Postgres with persistence, SSL and backups.

---

## 8 HTTP Error Handling Strategy

### 8.1 Key Decision: Error Safety vs Internal Details
**Problem**: API errors expose too much (500 errors leak internal details) or too little (generic messages aren't useful).

**Solution**: HTTPError interface separating user-safe messages from internal causes:
- Client errors (4xx): expose full details - validation errors are safe
- Server errors (5xx): hide internal details - "Internal Server Error" only  
- Logging gets full details via `Cause()` for debugging

### 8.2 Request Logging Decision  
**Problem**: No visibility into API request patterns, performance, or errors.

**Solution**: Middleware captures request/response lifecycle:
- Method, URI, status code, duration, byte counts
- Error details (from error context) for debugging
- Standard structured logging with slog

**Why not a framework**: Keep dependencies minimal, stdlib sufficient for current scope.

---

## 9 Future Roadmap
1. **Structured logging & tracing** – plumb `slog` with request IDs for better observability
2. **Throttling middleware for scraper** – optional, only if production back-fill shows 429s

---

That's the story so far—trimmed for length but (hopefully) not for laughs.

**Current Status**: Complete end-to-end system. Scraper processes ~760k delegations in ~15s with year column optimization. Web API serves real data via `GET /xtz/delegations` with GitHub-style pagination and year filtering. Database optimized with composite index for efficient queries.
