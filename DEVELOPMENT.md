
# Development Journey – Capturing the Why

This document is a living dev-diary recording **how** and **why** we built Delegator.  It preserves the experiments, dead-ends, and "aha!" moments that shaped the current code-base so reviewers can trace every decision back to evidence.

---

## 0 The Big Picture – “Screaming” Architecture
If you inspect the repo tree you’ll notice the directories _shout_ their purpose: `scraper`, `web`, `pkg`, `migrations`. That’s **Screaming Architecture**—package names that advertise **what** the system does rather than the frameworks it uses. A reviewer can skim `tree -L 2` and grasp the domain in seconds, no detective work required.

---

## 1 Environment & Tooling

- Go 1.24 – one language version to rule them all
- **Nix flakes + direnv** – `direnv allow && make run` spins up the whole world
- **Go workspace** – `scraper`, `web`, `pkg`, `migrations` live side-by-side without import hell
- **Makefile shortcuts** – `make run`, `make check`, `make help` (because nobody remembers long Docker commands)
- **12-Factor config** – every knob is an environment variable; `.env` just helps locally

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

---

## 3 Scraper Service (`scraper/`)

### 3.1 Problem Statement
Continuously ingest _every_ delegation exactly once and stay current, all while honouring the public API limits.

### 3.2 File Organization
Three files—`scraper.go` (contracts), `service.go` (engine), `subscriber.go` (loud-speaker)—keep code discoverable without a scavenger hunt. No table needed.

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
Event emission: BackfillStarted, BackfillDone, PollingStarted, etc.
```

The service now uses `Start()` which returns an events channel and done channel for clean shutdown:
- Events channel receives all lifecycle events (backfill, polling, errors)
- Done channel signals when service has completely shut down
- Context cancellation triggers graceful shutdown

### 3.5 Event Streaming
Seven event types provide complete observability:
* **BackfillStarted** / **BackfillSyncCompleted** / **BackfillDone** /**BackfillError** – backfill phase lifecycle
* **PollingStarted** / **PollingSyncCompleted** / **PollingShutdown** / **PollingError** – polling phase lifecycle

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
Switching from `pgx.Batch` to **`pgx.CopyFrom`** (with `ON CONFLICT DO NOTHING`) turned inserts into a fire-hose: **~50k rows/sec** on a laptop—about **7× faster** than the original batch approach. When reviewers run `make run-scraper` they’ll see the log line “persisted 10 000 rows in 180ms” and feel the speed. One helper, one temp table, zero hand-written SQL loops.

### 3.9 Key Take-Aways

- Unified `id.gt` pagination keeps back-fill and live polling on the same path.
- Event streaming decouples business logic from logging/observability.
- `CopyFrom` bulk insert gives **~7×** speed-up vs `pgx.Batch`.
- 10k-row chunks + 10s ticker stay well below free-tier limits.

### 3.10 Environment Variables
The scraper reads its settings from environment variables—​database DSN, chunk size, poll interval, etc. (see `end.demo` for the exhaustive list). 
```bash
make run-scraper        # full historical sync
```

---

## 4 Web API Service (`web/`)

### 4.1 Problem Statement
Expose collected delegation data via HTTP endpoint `GET /xtz/delegations` with pagination and year filtering.

### 4.2 Key Decisions
| Area | Decision | Rationale |
|------|----------|-----------|
| **Package layout** | `web/api` (contracts) → `web/handler/bind` (HTTP parsing) → `web/handler` (orchestration) | Keeps request/response shaping separate from business logic—easy to unit-test each slice. |
| **Pagination style** | GitHub-style `page`/`per_page` with `Link` headers | Familiar UX and simple to implement; no cursor complexity needed for demo scale. |
| **Error strategy** | Dual-path: user-safe messages on 4xx, generic message on 5xx + full details in structured logs | Protects internals while keeping developers productive and clients informed. |

### 4.3 Environment Variables
Configured entirely via env vars (DSN, host, port). Run locally with:
```bash
make run-web           # launch Web API
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

### 4.5 Database Optimization Decision
Dual indexes (timestamp DESC) and (year, timestamp DESC) turned multi-second scans into sub-millisecond look-ups—​a ~1,700× win.

**Pagination Implementation**: Uses LIMIT n+1 technique for efficiency - requests pageSize+1 records to detect "has more" without expensive COUNT queries. Simplified Link headers (rel="prev" and rel="next" only) match GitHub's actual API behavior, providing 35x performance gain by omitting COUNT-based "last" links.

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

---

## 6 Test Configuration Strategy

### 6.1 Key Decision – Module-Local `testcfg`
Each module owns a tiny `testcfg` package that parses **test-only** environment variables (all prefixed with `*_TEST_`). This lets acceptance tests dial chunk sizes down, shorten polling intervals, or point to sandbox endpoints **without touching** the developer’s normal `.env`.

```go
// scraper/testcfg/config.go (excerpt)
type Config struct {
    ChunkSize    uint64        `env:"SCRAPER_TEST_CHUNK_SIZE" envDefault:"1000"`
    PollInterval time.Duration `env:"SCRAPER_TEST_POLL_INTERVAL" envDefault:"100ms"`
}
```

Why bother? Parallel tests now finish in **under three seconds** instead of minutes, and because they point to throw-away template databases the developer’s own Postgres instance stays untouched.

### 6.2 Take-Away
Environment-configurable tests keep the codebase stateless, the suite lightning-fast, **and they help us sit comfortably at ~92% statement coverage** (see `make coverage`).
For the visual crowd, `make coverage-svg` pops up an interactive treemap so you can **see** which files need love at a glance.

### 6.3 Running Acceptance Tests: localhost vs. devcontainer
Acceptance tests (build tag `acceptance`) and `pgtestdb` both need a real Postgres reachable over the network. The only knob that differs between environments is **hostname**, driven by `SCRAPER_TEST_DATABASE_URL` (defaults to `localhost:5432`, same convention as `SCRAPER_DATABASE_URL`/`WEB_DATABASE_URL`/`MIGRATOR_DATABASE_URL`):

* **On the bare host** (`direnv allow && make check`): `docker compose up -d postgres` publishes `5432` to the host, so the `localhost` default just works.
* **Inside the devcontainer**: the `gopher-dev` service is its own container on the Compose network, so `localhost` there means *itself*, not `postgres`. `.devcontainer/docker-compose.yml` overrides every `*_DATABASE_URL` (including `SCRAPER_TEST_DATABASE_URL`) to `postgres:5432`, which resolves via Compose's built-in DNS — the same mechanism `docker-compose.yaml` already uses for the `migrator`/`scraper`/`web` service containers.

`migrator/migratortest.createTestDatabaseConfig` parses `SCRAPER_TEST_DATABASE_URL` (rather than hard-coding `localhost`) so `pgtestdb` picks up whichever host is correct for where the tests are running — no separate configuration or manual overrides needed in either environment. Run acceptance tests with `make test`/`make check`, or directly via `go test -tags=acceptance ./...` per module.

---

## 7 Where We’d Go Next (But Stopped Ourselves)
With another sprint we’d:
1. Swap the home-grown `httpkit` for an **OpenAPI-first** workflow: define the contract, generate server stubs (Echo), and expose a `/swagger` endpoint for free interactive docs.
2. Push structured logs to OpenTelemetry and wire up a Prometheus dashboard.
3. Add an exponential back-off helper for the rare 429s during back-fill.

But that’d drift beyond the “show me you can design, ship, and test a small system in Go” brief—so we parked it here.

---

Thanks for reading! Fire up `make run`, watch 760k delegations fly in, and poke `/xtz/delegations?page=1`. The code (and this diary) should answer the rest. 🥂

