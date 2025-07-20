# Delegator – Tezos Delegation Service Demo
A lightning-fast Go-based service that ingests **all** Tezos delegation operations and exposes them through a clean, paginated HTTP API.

**Why Delegator?**
* 🚀 **15s full sync**: Backfill 760k+ delegations in seconds
* ⚡ **<1 ms API**: Sub-millisecond responses with GitHub-style pagination
* 🧪 **92% test coverage**: Fast, real-world tests against the live TzKT API
* 🎯 **Exactly-once**: Resumable ingestion with graceful failure handling
* 💻 **One-command launch**: `make run` spins up everything with Docker Compose

---
## 🔧 Prerequisites
* **Go** ≥ 1.24
* **Docker** + **Docker Compose**
* **Nix + direnv** (optional for nix lovers :) 

## 🚀 Quick Start
💡 **Tip:** Run `make help` to see all available commands and targets.
### Full Data Sync
1. **Start all services** (migrator, scraper, web API, PostgreSQL): `make run` or `make run-demo`  for a lightweight demo dataset 
2. Watch the scraper in action: `docker compose logs -f scraper`
3. **Query the API:**
   * **Happy path:** `curl "localhost:8080/xtz/delegations?page=2&per_page=10&year=2025"`
   * **Error example:** `curl "localhost:8080/xtz/delegations?page=1&per_page=10&year=2100"`

## 🧪 Running Tests & Quality Gates
1. **Install dev tools** (first-time only): make deps
2. **Format, lint, and run tests**: make check
3. **View coverage**:
   * Text: `make coverage`
   * HTML: `make coverage-html` 
   * Visual treemap: `make coverage-svg`

## 📝 Log Samples
A sneak peek at the logs tail:  `docker compose logs -f <service>`:
### Scraper
```bash
scraper-1  | time="20.07.2025 12:38:38" level=INFO msg="Starting delegation scraper service" chunkSize=1000 version=main-029fe52 date=2025-07-20T12:38:28Z
scraper-1  | time="20.07.2025 12:38:38" level=INFO msg="Backfill started" startedAt="20.07.2025 12:38:38" checkpointID=1939557726552064
scraper-1  | time="20.07.2025 12:38:38" level=INFO msg="Backfill batch completed" fetched=1000 checkpointID=1954423887626240 chunkSize=1000
scraper-1  | time="20.07.2025 12:38:38" level=INFO msg="Backfill batch completed" fetched=471 checkpointID=1958496128991232 chunkSize=1000
scraper-1  | time="20.07.2025 12:38:38" level=INFO msg="Backfill completed" totalProcessed=1471 duration=290.905337ms
scraper-1  | time="20.07.2025 12:38:38" level=INFO msg="Polling started" interval=10s
scraper-1  | time="20.07.2025 12:38:48" level=INFO msg="Polling cycle completed, no new records"
```

### Migrator
```bash
migrator-1  | time="20.07.2025 12:38:37" level=INFO msg="Starting database migrator service" migrationsDir=/migrations initialCheckpoint=1939557726552064 version=main-029fe52 date=2025-07-20T12:38:28Z
migrator-1  | time="20.07.2025 12:38:37" level=INFO msg="Applying database migrations"
migrator-1  | time="20.07.2025 12:38:37" level=INFO msg="Database migrations applied successfully"
migrator-1  | time="20.07.2025 12:38:37" level=INFO msg="Initializing checkpoint" checkpoint=1939557726552064
migrator-1  | time="20.07.2025 12:38:37" level=INFO msg="Checkpoint initialized successfully"
migrator-1  | time="20.07.2025 12:38:37" level=INFO msg="Database migrator completed successfully"
```

### Web
```bash
web-1  | time="20.07.2025 12:38:38" level=INFO msg="Delegator Web API Service starting" version=main-029fe52 date=2025-07-20T12:38:28Z
web-1  | time="20.07.2025 12:38:38" level=INFO msg="Server started" addr=0.0.0.0:8080
web-1  | time="20.07.2025 12:41:04" level=INFO msg=HTTP method=GET uri="/xtz/delegations?page=1&per_page=10&year=2025" status=200 duration=3.812026ms bytes_in=0 bytes_out=1276
web-1  | time="20.07.2025 12:41:22" level=INFO msg=HTTP method=GET uri="/xtz/delegations?page=1&per_page=10&year=2017" status=400 duration=162.584µs bytes_in=0 bytes_out=63 error="invalid year: year out of valid range"
web-1  | time="20.07.2025 12:41:34" level=INFO msg=HTTP method=GET uri="/xtz/delegations?page=1&per_page=10&year=2100" status=400 duration=62.584µs bytes_in=0 bytes_out=63 error="invalid year: year out of valid range"
web-1  | time="20.07.2025 12:41:50" level=INFO msg=HTTP method=GET uri="/xtz/delegations?page=1&per_page=10000" status=400 duration=80.792µs bytes_in=0 bytes_out=101 error="invalid per_page: per_page exceeds maximum limit: must be between 1 and 100"
```

---

## 📖 Learn More

- **Exercise Brief**: [TASK.md](TASK.md)
- **Development Journey**: [DEVELOPMENT.md](DEVELOPMENT.md)
- **Requirements & Scope**: [REQUIREMENTS.md](REQUIREMENTS.md)
- **Architecture & Design**: [ARCHITECTURE.md](ARCHITECTURE.md)

---

## 📜 License

Released under the MIT License.
