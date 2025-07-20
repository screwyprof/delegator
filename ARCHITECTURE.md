# Tezos Delegation Service - System Design

---

## 1. System Overview

### 1.1 Purpose
The Tezos Delegation Service collects all delegation operations from the Tezos blockchain via the TzKT API and provides a high-performance HTTP API for querying this data with pagination and year-based filtering.

### 1.2 Core Requirements
- **Data Collection**: Ingest complete Tezos delegation history since 2018
- **API Service**: Serve `GET /xtz/delegations` with pagination and year filtering  
- **Performance**: Sub-second API responses, efficient bulk data processing
- **Reliability**: Exactly-once processing, resumable operations, graceful error handling
- **Observability**: Complete operational visibility through event streaming

### 1.3 Design Principles
- **CQRS Separation**: Independent read/write optimization
- **Event-Driven Architecture**: Decoupled observability and business logic
- **Clean Architecture**: Domain logic isolated from infrastructure concerns
- **Fail-Safe Design**: Graceful degradation and recovery patterns

---

## 2. System Architecture

### 2.1 High-Level Architecture

```
┌─────────────┐    ┌─────────────────┐    ┌─────────────┐
│   Migrator  │───▶│   PostgreSQL    │◀───│  Web API    │
└─────────────┘    │   (Delegations) │    └─────────────┘             
                   └─────────▲───────┘               
                             │                        
                   ┌─────────┴───────┐               
                   │    Scraper      │               
                   │  (TzKT Client)  │               
                   └─────────────────┘               
                             │                        
                   ┌─────────▼───────┐               
                   │   TzKT API      │               
                   │ (External Svc)  │               
                   └─────────────────┘               
```

### 2.2 Service Responsibilities

| Service | Role | Responsibility |
|---------|------|----------------|
| **Migrator** | Setup | Database schema creation, checkpoint initialization |
| **Scraper** | Write | TzKT API polling, data ingestion, event emission |
| **Web API** | Read | HTTP API serving, pagination, filtering |

### 2.3 Data Flow

1. **Schema Setup**: Migrator creates tables, indexes, and initial checkpoint
2. **Data Ingestion**: Scraper polls TzKT API → processes delegations → stores in PostgreSQL  
3. **Data Serving**: Web API queries PostgreSQL → formats JSON → returns paginated results

---

## 3. Component Design

### 3.1 Migrator Service
**Purpose**: Database lifecycle management  
**Runtime**: One-time execution per deployment  
**Key Features**:
- SQL migration application with versioning
- Demo/production checkpoint initialization
- Template database creation for testing

### 3.2 Scraper Service  
**Purpose**: Delegation data collection and processing  
**Runtime**: Long-running service with two phases  
**Architecture Pattern**: Event-driven with observable state transitions

**Processing Phases**:
```
Backfill Phase: Historical data collection from checkpoint
     ↓
Polling Phase: Continuous monitoring for new delegations  
```

**Key Features**:
- **Unified pagination**: `id.gt` filter for both backfill and live polling
- **Event streaming**: Lifecycle events enable observability, testing, and custom integrations
- **Chunked processing**: Configurable batch sizes (default: 10k records)
- **Checkpointing**: Resumable operations via last processed ID
- **Error handling**: Graceful failure with specific error categorization
- **Subscriber pattern**: Composable event handling for logging, monitoring, or custom actions
- **Pure business logic**: Event emission separates concerns from logging infrastructure

### 3.3 Web API Service
**Purpose**: High-performance delegation data access  
**Runtime**: Long-running HTTP service  
**Architecture Pattern**: Read-optimized with GitHub-style pagination

**API Design**:
```
GET /xtz/delegations?page=1&per_page=50&year=2025
```

**Key Features**:
- **Performance optimization**: LIMIT n+1 technique, dual-index strategy
- **Pagination**: GitHub-style with Link headers (rel="prev", rel="next")
- **Error handling**: Structured JSON errors with proper HTTP status codes
- **Request logging**: Comprehensive request/response middleware
- **Domain validation**: Value objects (`Page`, `PerPage`, `Year`) with rich validation
- **Clean architecture**: Separation of concerns across `api/`, `handler/`, `tezos/`, `store/` layers
- **Request-scoped error tracking**: HTTP context error propagation for observability

---

## 4. Data Architecture

### 4.1 Database Schema

```sql
-- Core delegation data
CREATE TABLE delegations (
    id BIGINT PRIMARY KEY,                   -- TzKT delegation ID
    timestamp TIMESTAMP WITH TIME ZONE,      -- Operation timestamp  
    amount BIGINT,                           -- Amount in mutez
    delegator TEXT,                          -- Sender address
    level BIGINT,                            -- Block height
    year INTEGER,                            -- Extracted for filtering
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Resumable processing checkpoint
CREATE TABLE scraper_checkpoint (
    single_row BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (single_row = TRUE),
    last_id BIGINT NOT NULL
);

-- Create standalone timestamp index for default queries without year filtering
CREATE INDEX IF NOT EXISTS idx_delegations_timestamp ON delegations (timestamp DESC); 

-- Create composite index for optimal year filtering and pagination
CREATE INDEX IF NOT EXISTS idx_delegations_year_timestamp ON delegations (year, timestamp DESC); 
```

**Performance Impact**: Direct year column filtering vs `EXTRACT(YEAR)` eliminates full table scans

### 4.2 Data Processing Pipeline

```
TzKT API → Domain Conversion → Bulk Insert → Checkpoint Update
    ↓             ↓                ↓             ↓
Field Selection  Validation    pgx.CopyFrom   Atomic Update
(~67% reduction) (Timestamp)   (7x faster)    (Resumability)
```

**Performance Optimizations**:
- **Field selection**: `select=id,timestamp,amount,sender,level` reduces payload by 67%
- **Temporary table pattern**: Bulk insert with `ON CONFLICT DO NOTHING` for duplicate safety
- **Connection pool tuning**: Production-optimized settings (2-10 connections, lifecycle management)
- **Checkpoint atomicity**: Singleton table pattern ensures consistent resumption state

### 4.3 Connection Management

**Production-Optimized PostgreSQL Connections**:
```bash
# Connection pool settings optimized for production
MinConns: 2              # Keep minimum connections warm
MaxConns: 10             # Reasonable max for most applications  
MaxConnLifetime: 30min   # Prevent stale connections
MaxConnIdleTime: 5min    # Close idle connections quickly
HealthCheckPeriod: 1min  # Regular health checks
ConnectTimeout: 10s      # Don't wait too long for new connections
```

**Benefits**: Prevents connection leaks, optimizes resource usage, ensures connection health

### 4.4 Error Handling

**Layered Error Management**:
- **API Layer**: Structured JSON errors with sanitized messages
- **Handler Layer**: Request-scoped error tracking  
- **Domain Layer**: Categorized sentinel errors
- **Store Layer**: Database-specific error classification

**Key Features**:
- 4xx errors expose validation details, 5xx errors use generic messages
- Original error context preserved for logging
- Request-scoped error propagation via context

---

## 5. Quality Attributes

### 5.1 System Characteristics
- **Data scale**: Processes complete Tezos delegation history (760k+ records)
- **API optimization**: 67% bandwidth reduction via selective field queries
- **Database optimization**: Bulk insert strategy (7x faster than individual operations)
- **Testing**: Sub-3-second test suite execution

### 5.2 Reliability Features
- **Exactly-once processing**: Database constraints prevent duplicates
- **Resumable operations**: Checkpoint-based recovery after failures  
- **Graceful shutdown**: Context-based cancellation with cleanup
- **Error categorization**: Specific error types for different failure modes

### 5.3 Security Features
- **Error sanitization**: Internal errors use generic messages in API responses
- **Security headers**: `X-Content-Type-Options: nosniff` automatically added
- **Information isolation**: Sensitive error details available only in logs

### 5.4 Observability
**Event-Driven Approach**: Business logic emits events, infrastructure handles logging
- **Lifecycle visibility**: Events cover all service state transitions
- **Deterministic testing**: Events provide precise synchronization points
- **Structured logging**: JSON format with version and context information

### 5.5 Testing Strategy
**Template Database Pattern**: Fast test execution using database template cloning
- **Real-world validation**: Acceptance tests against actual TzKT API and PostgreSQL
- **Event-driven synchronization**: Deterministic test timing via business events
- **High coverage**: 92% test coverage with sub-3-second execution
- **Environment isolation**: Independent test configuration and parallel execution

---

## 6. Current Implementation Status

### 6.1 Current Status
- Three-service architecture operational (migrator, scraper, web API)
- Processes real Tezos delegation data from TzKT API
- 92% test coverage with acceptance tests
- Structured logging and graceful shutdown implemented
- Version injection and build metadata

### 6.2 Current Limitations
- No metrics endpoints or health checks
- Development storage (tmpfs) not production-ready
- No circuit breakers or sophisticated retry strategies  
- No authentication or TLS termination

### 6.3 Configuration Management

All services use environment variables for configuration following 12-factor app principles. 

**Complete configuration reference**: See `env.demo` file for all environment variables with defaults and examples.

---

## 7. Technology Stack

### 7.1 Core Technologies
- **Language**: Go 1.24+ (workspace, modules, generics)
- **Database**: PostgreSQL 16 with pgx driver  
- **HTTP**: Standard library with custom middleware
- **Testing**: Testify, pgtestdb for database testing
- **Configuration**: Environment variables (caarlos0/env)

### 7.2 External Dependencies
- **TzKT API**: Tezos blockchain data source (10 rps free tier)
- **PostgreSQL**: Primary data store with optimized configuration
- **Docker**: Containerization and local development

### 7.3 Development Tools
- **Quality**: golangci-lint, gofumpt, race detector
- **Testing**: Acceptance tests with real external dependencies
- **Automation**: Makefile-driven development workflow
- **Development environment**: Nix flakes for reproducible toolchain

---

## 8. Deployment Considerations

### 8.1 Operational Dependencies
- **PostgreSQL**: Persistent storage with backup strategy
- **TzKT API**: External service availability (99.9% typical)
- **Container Runtime**: Docker 20.10+ or equivalent

### 8.2 Current Observability
**Available Observability**:
- Structured JSON logging with lifecycle events
- Service startup/shutdown logging
- Business logic events (7 event types for scraper)
- Request/response logging for web API

**Future Monitoring** (see Evolution Roadmap):
- Health endpoints and metrics
- Alerting and dashboards
- Advanced observability tooling

---

## 9. Twelve-Factor App Compliance

The system follows [twelve-factor app](https://12factor.net) methodology with **11/12 compliance**:

**✅ Compliant**: Environment-based configuration, stateless processes, explicit dependencies, backing services as resources, build/release/run separation, port binding, disposability, dev/prod parity, logs as streams, admin processes as separate services

**⚠️ Limitation**: Factor VIII (Concurrency) - scraper cannot be horizontally scaled due to shared checkpoint design

---

## 10. Evolution Roadmap

### 10.1 API & Documentation
- **OpenAPI schema-first design** with Swagger UI (`/swagger`)
- **API versioning strategy** (e.g., `/v1/xtz/delegations` and/or with version headers)
- Enhanced error documentation and examples

### 10.2 Operations & Monitoring
- Health endpoints (`/healthz`, `/ready`)
- Prometheus metrics (`/metrics`)
- Distributed tracing (OpenTelemetry)
- Alerting and dashboards

### 10.3 Resilience & Reliability
- Basic circuit breakers for TzKT API
- Retry strategies with exponential backoff
- Rate limiting and throttling
- Persistent storage configuration

### 10.4 Security & Performance
- Security features (TLS, auth, input validation)
- Cursor-based pagination for large datasets
- Read replicas and connection pooling optimization

### 10.5 Architecture Evolution
- Worker pool architecture for parallel processing
- Event-driven pipeline with normalizer service
- Horizontal scaling capabilities

---

This system design documents the current implementation and provides clear evolution paths for enhanced capabilities. 