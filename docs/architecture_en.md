# Architecture Design

This document describes the overall architecture of the `search-engine` project, the key
design decisions, and the reasoning behind them. For package-level detail see
`package-structure_en.md`; for setup/run instructions see `local-setup_en.md`.

![Architecture flow diagram](architecture-flow.png)

![Architecture overview](architecture-overview.png)

## 1. Overview

The service is built around three main flows:

1. **Ingestion** — periodically fetches data from three content providers (2 real + 1
   self-authored mock), normalizes it, writes it to MongoDB, and indexes it into
   Elasticsearch.
2. **Search** — a user query finds candidates in Elasticsearch, which are then re-ranked by a
   custom scoring formula inside the application and paginated.
3. **Observability** — both flows emit metrics/logs; Prometheus scrapes them, Grafana
   visualizes them, and Alertmanager raises alerts on critical conditions.

## 2. High-level data flow

```
providers (JSON x2, XML x1)
        │  Resilient (timeout + retry + circuit breaker)
        ▼
  ingestion.Job.RunAll
        │  UpsertMany                       │ FindUnindexed (Reconcile)
        ▼                                    │
     MongoDB  ◀──────────────────────────────┘
   (source of truth)
        │  IndexMany
        ▼
  Elasticsearch (search view)
        │  Search (BM25 recall)
        ▼
  search.Service (scoring + pagination)
        │
        ▼
  HTTP API (/search, /refresh, /metrics)
        │
        ▼
  Frontend (React, Vite)

  app:/metrics ──▶ Prometheus ──▶ Alertmanager
                       │
                       ▼
                    Grafana ◀── Loki (container logs collected via Promtail)
```

## 3. Why MongoDB + Elasticsearch (a CQRS-like split)

- **MongoDB = source of truth.** Every record from every provider is upserted keyed on
  `external_id + provider` (idempotent). Provider-specific fields that don't fit the common
  schema are preserved in `RawFields` rather than being dropped.
- **Elasticsearch = a derived, rebuildable search view.** Only the fields needed for search
  and ranking are indexed; `RawFields` is deliberately not indexed. ES is never the source of
  authority — even if the index were deleted, it could be rebuilt end-to-end from Mongo.
- This split is a simplified instance of CQRS's "write model" / "read model" separation:
  writes go to Mongo, reads (search) go to Elasticsearch.

## 4. Resilience layer

`internal/provider/resilient.go` wraps every provider with a Decorator:

- **Timeout** — an upper bound on every fetch call (`FetchTimeout`, default 5s).
- **Retry + exponential backoff** (`sethvargo/go-retry`) — 3 attempts on transient failures.
- **Circuit breaker** (`sony/gobreaker`) — trips to **Open** once
  `ConsecutiveFailures > 5`; after `Timeout=10s` it allows one attempt in **HalfOpen**;
  `MaxRequests=3` consecutive successes close it back to **Closed**.

Why it matters: if one provider goes down, the other providers' ingestion cycle is
unaffected, and the failing provider isn't hammered with pointless requests — while the
breaker is open, calls are rejected immediately without ever reaching the network
(`"circuit breaker is open"`).

## 5. Data consistency: MongoDB → Elasticsearch (a simplified outbox)

**Problem**: writing to Mongo and indexing into Elasticsearch are two separate,
non-atomic operations (the "dual write" problem). If the Mongo write succeeds but the ES
index fails, the data exists but can't be found by search.

**Implemented solution** (`domain.Content.Indexed` + `IndexedAt`):

- Every `Content` document carries an `indexed: bool` field; every newly written record
  starts as `indexed=false`.
- `ingestion.Job.runOne` attempts to index into ES right after the Mongo write; on success
  it calls `MarkIndexed` to set `indexed=true`.
- At the end of every ingestion cycle (`RunAll`), `Reconcile()` runs: it finds records still
  `indexed=false` (up to `reconcileBatchSize=1000`) and retries indexing them into ES.
- As a result, the inconsistency window is not permanent — it's **at most as long as the
  next ingestion cycle**.
- `content_pending_indexing` (a Prometheus gauge) makes this backlog observable; the
  `ContentIndexingBacklog` alert rule fires if the backlog doesn't clear for 2 minutes.

**Why not a full outbox pattern / Kafka**: a true outbox pattern requires writing an
"outbox event" alongside the content write in the same transaction, consumed by a separate
worker (or a CDC tool like Debezium tailing MongoDB's oplog and publishing to Kafka). At this
project's scale — a single-node MongoDB, a single consumer, a single event type — that would
be over-engineering (transactions require a replica set; Kafka/Debezium add real operational
overhead). Instead, the same idea is applied in a **simplified, single-collection** form: the
document itself plays double duty as both the data and the "pending event."

## 6. Search and scoring

- Elasticsearch is purely a **recall/filter** layer — `candidateWindowSize=500` retrieves
  the top 500 BM25-relevant candidates.
- Final ranking is not done by the BM25 score but by the custom formula in
  `internal/scoring`: `Final Score = Base Score × Type Coefficient + Freshness Score +
  Engagement Score`. This "retrieve then re-rank" pattern separates search relevance from a
  business-rule-driven popularity/freshness ranking.
- `internal/search/service.go` sorts, paginates (`normalizePaging`) and validates
  (`validateQuery`) the request. Deliberate design distinction: invalid search input (empty
  or too-long keyword, invalid type) is **not silently corrected** — it's rejected with
  `ErrInvalidQuery` and a `400`; pagination parameters (`page`, `page_size`), by contrast,
  **are silently** clamped to safe defaults, since those are usually harmless values from a
  UI's initial render rather than a genuine client error.

## 7. Observability

- **Metrics** (`internal/observability/metrics`): `provider_circuit_state`,
  `provider_requests_total`, `provider_request_duration_seconds`,
  `search_request_duration_seconds`, `content_pending_indexing`. Prometheus scrapes these
  from `/metrics` every 10 seconds.
- **Logging** (`internal/observability/logger`): JSON structured logging via `slog`; every
  HTTP request is tagged with a `request_id` (UUID, `middleware/request_id.go`), enabling
  end-to-end log tracing by that ID in Loki.
- **Dashboards** (Grafana): Go runtime metrics (heap, goroutines, CPU) and a Provider Health
  panel (circuit breaker state, request rate, p95 latency).
- **Alerting** (Prometheus + Alertmanager): 5 rules — `ProviderCircuitBreakerOpen`,
  `HighSearchLatency`, `SearchEngineAPIDown`, `HighProviderFailureRate`,
  `ContentIndexingBacklog`. Prometheus **evaluates** rules, Alertmanager **groups/routes**
  notifications — a deliberate separation of concerns. There's currently no real notification
  channel (Slack/email); `alertmanager.yml`'s `receivers` block is intentionally a null
  receiver — that's the only piece that would change in production.

## 8. Robustness and operational maturity

- **Graceful shutdown** (`cmd/api/main.go`): `SIGINT`/`SIGTERM` are caught
  (`signal.NotifyContext`), the HTTP server is given 15 seconds to finish in-flight requests
  (`server.Shutdown`), then the Mongo connection is closed cleanly.
- **Rate limiting** (`internal/transport/http/middleware/ratelimit.go`): a dependency-free,
  hand-written per-IP token bucket; applied only to `/search` and `/refresh` — deliberately
  not to `/metrics`, so Prometheus's regular scrape isn't affected.
- **Input validation**: `search.Service.validateQuery` rejects clearly invalid input outright
  (see section 6).
- **Tests**: table-driven unit tests for `internal/search`, `internal/ingestion`, and
  `internal/provider`, using fake/stub implementations to test in isolation without a real
  Mongo/ES/HTTP dependency (see the test file list in `package-structure_en.md`).

## 9. Tech stack

**Backend (Go 1.27)**

| Library | Version | Purpose |
|---|---|---|
| `go.mongodb.org/mongo-driver` | v1.17.1 | MongoDB client |
| `github.com/elastic/go-elasticsearch/v8` | v8.15.0 | Elasticsearch client |
| `github.com/sony/gobreaker` | v0.5.0 | Circuit breaker |
| `github.com/sethvargo/go-retry` | v0.3.0 | Retry + exponential backoff |
| `github.com/prometheus/client_golang` | v1.20.5 | Prometheus metrics client |
| `github.com/google/uuid` | v1.6.0 | `request_id` generation |
| `log/slog` (standard library) | — | Structured (JSON) logging |

**Frontend**

| Technology | Version | Purpose |
|---|---|---|
| React | 18.3 | UI library |
| TypeScript | 5.6 | Type safety |
| Vite | 5.4 | Build tool + dev server |
| Tailwind CSS | 3.4 | Utility-first styling |
| Framer Motion | 11 | Animations (result cards, transitions) |
| Nginx | 1.27-alpine | Static file serving in production |

**Infrastructure (orchestrated via Docker Compose)**

| Service | Image/Version | Role |
|---|---|---|
| MongoDB | `mongo:7` | Source of truth |
| Elasticsearch | `docker.elastic.co/elasticsearch/elasticsearch:8.15.0` | Search index |
| Prometheus | `prom/prometheus:v2.55.0` | Metrics scraping + alert evaluation |
| Alertmanager | `prom/alertmanager:v0.27.0` | Alert grouping/routing |
| Grafana | `grafana/grafana:11.2.0` | Dashboard visualization |
| Loki + Promtail | `grafana/loki:3.1.0` + `grafana/promtail:3.1.0` | Log collection/querying |
| Mongo Express | `mongo-express:1.0.2` | Browse Mongo data (dev tool) |
| Elasticvue | `cars10/elasticvue:1.16.0` | Browse ES data (dev tool) |
| Go build image | `golang:1.27-alpine` → `alpine:3.20` | Multi-stage Docker build |
| Node build image | `node:20-alpine` | Frontend build stage |

## 10. Summary of deliberate trade-offs

| Decision | Alternative | Why this was chosen |
|---|---|---|
| `indexed` flag + `Reconcile` | Full outbox + Kafka/Debezium | Would be over-engineering at this scale |
| Hand-written rate limiter | `golang.org/x/time/rate` | To keep the mechanism transparent in the code itself |
| BM25 (recall) + application-side re-rank | ES's own ranking only | The scoring formula is part of the spec and must not be conflated with ES's relevance score |
| Null receiver in Alertmanager | Real Slack/email integration | Out of scope, but the infrastructure was built to demonstrate the "rule evaluation vs. notification routing" separation |
| cAdvisor on Docker Desktop | Container-level CPU/RAM metrics | Didn't work reliably due to a mount-propagation limitation in Docker Desktop's LinuxKit VM, so it was removed — only Go runtime metrics were kept |