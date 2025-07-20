# Tezos Delegation Service – Requirements (Green-field Draft)

This document captures _what_ the service **must** do (“requirements”) and _what is deliberately left out_ for the first delivery. It defines the initial scope derived solely from the exercise brief in [TASK.md](TASK.md) and general software-engineering best practices.

---

## 1. Goals

* Deliver a self-contained service that collects **all Tezos delegation operations** and exposes them via a public HTTP API.
* Showcase clean software design, test-driven development, and pragmatic **Go** craftsmanship within a single repository that can be evaluated quickly by reviewers.
* Run locally with minimal setup (Makefile + Docker Compose) and avoid overwhelming the upstream TzKT API (exact rate limits to be confirmed during implementation).

---

## 2. Functional Requirements

1. **Delegation Collector (“Scraper”)**
   * Poll the endpoint `GET /v1/operations/delegations` on `https://api.tzkt.io`.
   * Persist, for every delegation operation:
     * `timestamp` (ISO-8601 UTC)
     * `amount` (mutez, kept as integer string)
     * `delegator` (sender’s address)
     * `level` (block height)
   * Back-fill **all historical data since 2018** on first run, then continue **live polling** indefinitely.
   * Ensure **exactly-once** persistence (no duplicates after restarts).
   * Maintain a **checkpoint** (last processed delegation ID) so the collector resumes seamlessly after restarts or crashes.

2. **Public API (“Web”)**
   * Expose `GET /xtz/delegations` that returns the recorded delegations as JSON.
   * Default ordering: **most recent first**.
   * Support optional query parameter `year=YYYY` to filter by calendar year.
   * Implement pagination with a default **page size of 50** and an upper cap of 100.
   * (nice-to-have) Expose a lightweight **health endpoint** (`/healthz`) that returns 200 OK once the service is ready – useful for container orchestration and local scripts.
   * (nice-to-have) Provide an **OpenAPI (Swagger) specification** so clients can easily integrate.

3. **Database Schema & Migration**
   * Provide SQL scripts (or an equivalent mechanism) to create required tables and indexes.
   * The exact bootstrap approach (manual `psql`, container init script, or a small helper CLI) will be chosen during implementation.

4. **Configuration & Operations**
   * All runtime settings supplied via **environment variables** with sensible defaults.
   * Graceful shutdown on SIGINT/SIGTERM with in-flight work completion.

---

## 3. Non-Functional Requirements

| Category | Requirement |
|----------|-------------|
| **Reliability** | Continuously processes new delegations without manual intervention; resumes after crashes using checkpoints. |
| **Performance** | End-to-end latency for new delegations ≤ 1 min; API must serve typical queries in < 100ms on a laptop. |
| **Scalability** | Designed for demo scale. No horizontal scaling required, but code should avoid obvious bottlenecks (N + 1 queries, etc.). |
| **Observability** | Structured logging: **JSON** in production images, **human-readable text** in local/dev runs; INFO level default with DEBUG toggle. |
| **Security** | Expose only the public endpoint; no authentication in scope. |
| **Portability** | Runs on macOS/Linux with Docker installed; tested on Go ≥ 1.24. |
| **Maintainability** | Idiomatic Go 1.24, small focused packages, clear boundaries (**CQRS** between write/read paths). |
| **Testability** | ≥ 80% statement coverage across units + acceptance tests; deterministic tests runnable with `make test`. |

---

## 4. Out-of-Scope (Won’t-Have-Now)

* **Advanced resilience** – circuit breakers, retries, SLA dashboards.
* **Full production hardening** – TLS termination, authentication, and similar features.
* **Autoscaling & high availability** – single replica per service is acceptable.
* **Historical analytics endpoints** – only the single delegations endpoint is required.
* **Message queues / event streaming** – database coupling is acceptable for the demo.
* **Concurrent worker pools / parallel fetchers** – unnecessary for the small data volume; single-threaded logic keeps the code simpler.
* **Fully event-driven pipeline with normaliser service & dedicated read store** – common in production for CQRS or microservices architectures but beyond demo scope.
* **Deep observability** – distributed tracing, Prometheus/Grafana metrics, centralized log aggregation.
* **Performance tuning & ultra-low latency** – fine-grained database tuning, caching layers, connection pools beyond the defaults.

> _In a production deployment_ one would also expect health probes, structured metrics, tracing, autoscaling policies, graceful zero-downtime upgrades, secret management, and compliance controls (audit logs, encryption at rest, etc.). These are acknowledged but intentionally deferred outside the demo scope.

These items may appear in a _future evolution_ document but are not built for the initial delivery.

---

## 5. Proposed System Architecture

```
┌─────────────────┐    poll    ┌──────────────┐
│     Scraper     ├──────────► │  TzKT API    │
└────────┬────────┘            └──────────────┘
         │ write                              
         ▼                                     
┌─────────────────┐    read   ┌──────────────┐
│  PostgreSQL     | ◄─────────┤    Web API   │
└─────────────────┘           └──────────────┘
```

* **Scraper** – Stateless worker that streams delegations into PostgreSQL.
* **Web API** – Read-only service that queries PostgreSQL and formats JSON responses.
* Local orchestration via **Docker Compose** starts both services plus a Postgres container. Database schema is applied during container startup using simple SQL scripts.

---

## 6. High-Level Project Structure

```
delegator/
├─ cmd/                  # Service entry points
│  ├─ scraper/           # Scraper service main()
│  └─ web/               # Web API service main()
├─ scraper/              # Scraper – fetch TzKT data, map & persist to DB
├─ web/                  # Web API handlers, binders, DB access
├─ migrations/           # Raw SQL migration files
├─ docker-compose.yml    # Local orchestration
└─ Makefile              # Developer commands
```

A top-level **Go workspace** ties the modules together while preserving clean boundaries. Keeping everything in a **single monorepo** makes it trivial to share code, run cross-module tests, and mirrors how large organisations (e.g. Google, Uber) structure Go projects.

---

## 7. Quality & Delivery Pipeline

| Stage | Tooling | Gate |
|-------|---------|------|
| Format | `gofumpt` | must pass locally |
| Lint   | `golangci-lint` (govet, staticcheck, revive, etc.) | No warnings |
| Test   | `go test -race ./...` | ≥ 80% coverage |
| Build  | `docker build` for each service | Image size < 100 MB |
| Acceptance | End-to-end tests spinning real Postgres & hitting live TzKT API (skipped in short CI) | All green |
| Commit history | Conventional Commits + Semantic Versioning | Manual discipline; commit hooks if time permits |
| Security | `golangci-lint` (includes `go vet`); optional container image scan | Informational only |

`make check` runs the full quality gate locally. Optional pre-commit hooks can run a faster subset before each commit.

---

## 8. Developer Experience

* **Single-command run** – `make run` launches Postgres + all services.
* **Configuration via environment variables** – adhering to the Twelve-Factor App principle; a `.env` file is provided for local convenience.
* **Acceptance-test-driven development (ATDD)** – each slice of functionality begins with a high-level acceptance test, followed by short **TDD** cycles to drive the implementation details.
* **Rich documentation** in `README.md`; change log and design rationale captured in `DEVELOPMENT.md`.

---

## 9. Initial Success Criteria

1. After running `make run`, the default first page (`/xtz/delegations?page=1`) returns a non-empty JSON payload within a few minutes on a typical laptop (full historical back-fill can continue unobtrusively in the background).
2. `make test` passes with race detector enabled.
3. An automated local pipeline (format → lint → test → build images) runs green.

---

## 10. Assumptions & Risks

* **External API stability** – The public TzKT endpoint remains reachable and its response schema does not change unexpectedly.
* **Rate limiting** – Actual limits are unknown at design-time; the scraper will employ polite polling intervals and back-off on HTTP 429.
* **Network throughput** – A slow connection may stretch the first full sync beyond “a few minutes”, but the API becomes useful early as data arrives incrementally.
* **Local resources** – A modern developer laptop (at least ≈4-core CPU, 8GB RAM) — we’re not targeting a dusty Pentium 4 from 2005 😄.

---

## 11. Licence

This project is released under the **MIT License**.


---

## 12. Glossary

* **Delegation** – Operation on Tezos where a token holder delegates staking rights.
* **TzKT** – Public blockchain explorer & API for Tezos.
* **Mutez** – Smallest Tezos currency unit (1 XTZ = 1 000 000 mutez).
* **CQRS** – Command-Query Responsibility Segregation; separates write and read models.
