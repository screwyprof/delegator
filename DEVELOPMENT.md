# Development Journey – Capturing the Why

This document is a living dev-diary recording **how** and **why** we built Delegator.  It preserves the experiments, dead-ends, and "aha!" moments that shaped the current code-base so reviewers can trace every decision back to evidence.

---

## 1 Environment & Tooling

- Go 1.24 (single version across the repo)
- Nix flakes + direnv: `direnv allow && make run`
- Single Go workspace: `scraper`, `web`, `pkg`
- Makefile: `make help`, `make check`, `make run`

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

## 4 Database Configuration Strategy

Dev: tmpfs Postgres for quick iterations.  
Prod: standard Postgres with persistence, SSL and backups.

---

## 6 Testing & Coverage Strategy

• Unit tests for domain logic.  
• Acceptance test hits real Tzkt API + Postgres.  
• `make test` runs everything with race detector and parallelism.  
Coverage target: ≥80 % (store integration covered by acceptance test).

---

## 7 Future Roadmap
1. **Web API** – expose `GET /xtz/delegations` (will live in `web/`).
2. **Structured logging & tracing** – plumb `slog` with request IDs once HTTP entry points appear.
3. **Throttling middleware** – optional, only if production back-fill shows 429s.

---

That's the story so far—trimmed for length but (hopefully) not for laughs.

**Current Status**: the scraper chugs through ~760 k delegations in ~15 s. Next up: the Web API.
