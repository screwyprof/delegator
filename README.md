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
* Importing the entire historical delegation dataset.
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
* Startup back-fill: last 1 000 delegations or 14 days of history.
* Chunked fetch (`limit=500`) + rate limiter (`SCRAPER_RPS_LIMIT`).
* Retries with back-off; stores `LAST_PROCESSED_ID` checkpoint.

### Web API
* Endpoint `GET /xtz/delegations` with `year` filter and 50-item pagination.
* JSON errors `{ "code": n, "error": "msg" }`.
* Listens on `HTTP_PORT` (8080).

---

## Testing
* **Unit + Acceptance** – `make test` (race, verbose, coverage ≥ 80%).
* Quality gates – `make check`

---
