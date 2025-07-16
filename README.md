# Delegator – Tezos Delegation Service

A Go-based service that collects Tezos delegation data and serves it through a public
API.

> **TL;DR**  
> • Scraper → Postgres ← Web API  
> • `make run` starts the whole stack  
> • Fits Tzkt free-plan limits (10 req/s, 500 k req/day)

---

## Quick start
```bash
$ make run # start scraper, web API and PostgreSQL
$ curl localhost:8080/xtz/delegations?page=1
```

Requirements: Go 1.24+, Docker + Compose.

---

## Goals
* Deliver an end-to-end delegation flow demo with two Go services and a shared database.
* Stay inside the free Tzkt API tier (≤10 rps, 500 k requests/day).
* Maintain simple, readable code with ≥80% test coverage and passing lint/format gates.
* Showcase a clear Read/Write sides split: Scraper (write) and Web API (read).


## Non-Goals
* High-availability & resilience.
* Event-driven ETL pipelines.
* Production-grade instrumentation.

---

## Architecture & Scope
See the full specification in [REQUIREMENTS.md](REQUIREMENTS.md).  

### Highlights
```
Scraper → PostgreSQL ← Web API
```

* **CQRS split** – scraper handles write-heavy ingestion; Web API handles read-heavy queries.  
* **Single DB for demo** – simple to run; future evolution adds Normalizer + separate read DB 

### Scraper
* Startup back-fill: controlled via checkpoint system (0 = all data, higher ID = demo subset).
* Chunked fetch (`limit=10000`) + natural rate limiting for live polling (10s intervals).
* Retries with back-off; stores `LAST_PROCESSED_ID` checkpoint.
* **Production validated**: successfully processes real Tezos delegation data.

### Web API
* Endpoint `GET /xtz/delegations` with `year` filter and 50-item pagination.
* JSON errors `{ "code": n, "error": "msg" }`.
* Listens on `HTTP_PORT` (8080).

---

## Testing
* **Unit + Acceptance** – `make test` (race, verbose) and `make coverage` (with console text and HTML report).
* **Testing patterns** – follows behavior testing, parallel execution, proper helper functions.
* **Deterministic synchronization** – direct method testing instead of timeout-based coordination.
* Quality gates – `make check`

## Production Status
* ✅ **Data collection**: Successfully fetches and processes real Tezos delegation data
* ✅ **API integration**: Handles both small and large responses with proper gzip decompression  
* ✅ **Rate limiting**: Live polling naturally respects API limits (10s intervals). Backfill documents limitation for production use.

---
