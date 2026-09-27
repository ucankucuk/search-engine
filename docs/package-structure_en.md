# Package Structure and Best-Practice Basis

This layout follows [`golang-standards/project-layout`](https://github.com/golang-standards/project-layout)
— the most widely referenced community layout in the Go ecosystem — and three principles that
recur across current (2026) Go architecture writing:

1. **`cmd/` is only an entry point.** No business logic lives there; it just wires up
   `internal/` packages together (the "composition root").
2. **Inside `internal/`, packages are organized by domain/responsibility, not by type.** Models
   aren't dumped into one shared `models/` folder; each package owns its own responsibility
   (provider access, search, scoring, HTTP transport...). `internal/` itself is a rule enforced
   by the Go compiler: packages inside it can never be imported from outside the project.
3. **Dependencies point inward** (Go's take on clean architecture). The `domain` package imports
   nothing; outward-facing packages like `provider`, `storage`, and `search` import `domain` but
   not each other; `cmd/api` sits at the very outside and wires everything together. This
   project doesn't use a `pkg/` folder because there's no code meant to be shared as a library
   with another project — best-practice sources generally treat `pkg/` as an unnecessary no-op
   layer in that case.

## Directory tree

```
search-engine/
├── go.mod, go.sum
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── README.md                           # EN
├── README_tr.md                        # TR
├── docs/
│   ├── architecture_tr.md / architecture_en.md
│   ├── package-structure_tr.md / package-structure_en.md
│   ├── local-setup_tr.md / local-setup_en.md
│   ├── architecture-flow.png
│   └── architecture-overview.png
├── cmd/
│   └── api/
│       └── main.go                    # composition root + graceful shutdown
├── internal/
│   ├── domain/
│   │   ├── content.go                 # Content, ScoredContent, SearchQuery/Result, Indexed flag
│   │   └── errors.go
│   ├── config/
│   │   └── config.go                  # every env variable, in one place
│   ├── provider/
│   │   ├── provider.go                # Provider interface + registry
│   │   ├── fetcher.go                 # RawFetcher / Normalizer interfaces
│   │   ├── adapter.go                 # Adapter (combines RawFetcher+Normalizer)
│   │   ├── resilient.go               # timeout + retry + circuit breaker decorator
│   │   ├── resilient_test.go
│   │   ├── json/
│   │   │   ├── fetcher.go
│   │   │   └── normalizer.go
│   │   └── xml/
│   │       ├── fetcher.go
│   │       └── normalizer.go
│   ├── ingestion/
│   │   ├── job.go                     # RunAll / RunOne / Reconcile
│   │   ├── job_test.go
│   │   └── scheduler.go               # periodic trigger (time.Ticker)
│   ├── storage/
│   │   ├── mongo/
│   │   │   └── repository.go          # UpsertMany, FindUnindexed, MarkIndexed
│   │   └── elastic/
│   │       └── index.go               # IndexMany, Search, index mapping
│   ├── scoring/
│   │   └── scoring.go                 # pure scoring formula
│   ├── search/
│   │   ├── service.go                 # search business logic + validation + pagination
│   │   └── service_test.go
│   ├── transport/
│   │   └── http/
│   │       ├── router.go              # path → handler → middleware chain
│   │       ├── handler/
│   │       │   ├── search.go
│   │       │   └── refresh.go
│   │       └── middleware/
│   │           ├── request_id.go      # UUID + structured request logging
│   │           ├── cors.go            # single-origin CORS
│   │           └── ratelimit.go       # per-IP token-bucket rate limiter
│   └── observability/
│       ├── logger/
│       │   └── logger.go              # slog setup (JSON, stdout)
│       └── metrics/
│           └── metrics.go             # single definition point for every Prometheus metric
├── mocks/
│   └── json-provider-a/               # our own third (JSON) mock provider
│       ├── main.go
│       ├── go.mod                     # a separate Go module — builds independently of the main project
│       └── Dockerfile
├── frontend/                          # React + TypeScript + Vite + Tailwind + Framer Motion
│   └── src/
│       ├── App.tsx, main.tsx, index.css, types.ts
│       ├── api/client.ts              # HTTP calls to the backend
│       ├── components/                # SearchBar, FilterBar, ResultList, ResultCard,
│       │                              # ScoreBar, TypeBadge, Pagination, EmptyState, ...
│       └── hooks/useDebounce.ts
└── deployments/
    ├── prometheus/
    │   ├── prometheus.yml             # scrape config + rule_files + alerting.alertmanagers
    │   └── rules.yml                  # 5 alert rules
    ├── alertmanager/
    │   └── alertmanager.yml           # route + null receiver
    ├── grafana/provisioning/
    │   ├── datasources/datasources.yml
    │   └── dashboards/
    │       ├── dashboards.yml
    │       ├── provider-health.json
    │       └── system-health.json
    ├── loki/loki-config.yml
    └── promtail/promtail-config.yml   # ships container logs into Loki
```

## What each package is for

### `cmd/api`
**main.go** — the application's single entry point (composition root). Reads config, opens the
Mongo/Elasticsearch connections, registers providers, applies the resilience layer, wires up
services and HTTP handlers, and starts the server. **Graceful shutdown** lives here:
`SIGINT`/`SIGTERM` are caught, the HTTP server gets 15 seconds to finish in-flight requests,
then the Mongo connection is closed cleanly. **It contains no business rules** — only
information about which piece connects to which.

### `internal/domain`
Holds the application's core data models (`Content`, `ScoredContent`, `SearchQuery`,
`SearchResult`) and shared error types (`ErrContentNotFound`, `ErrInvalidQuery`). **It imports
nothing** — this is the Go equivalent of the "entities" layer in clean architecture. `Content`
also carries `Indexed`/`IndexedAt` — internal bookkeeping fields for the Mongo↔ES consistency
mechanism (a simplified outbox, see `architecture_en.md` section 5) that never leak into the
API response (`json:"-"`).

### `internal/config`
Reads environment variables into a type-safe `Config` struct. No other package in the project
calls `os.Getenv` directly. `RateLimitRPS`/`RateLimitBurst` configure the rate limiter.

### `internal/provider`
Abstracts access to external content providers behind a `Provider` interface.
- `provider.go`: the `Provider` interface plus a self-registering registry (`Register`, `All`,
  `Get`).
- `fetcher.go`: the `RawFetcher` (connection) and `Normalizer` (format conversion) interfaces —
  "how to connect" is deliberately kept separate from "how to normalize."
- `adapter.go`: `Adapter`, which combines a `RawFetcher` and a `Normalizer` into a full
  `Provider`.
- `resilient.go`: `Resilient`, a decorator that wraps any `Provider` with timeout +
  retry/backoff + circuit breaker.

### `internal/provider/json`, `internal/provider/xml`
The concrete implementation for each provider family (the actual HTTP call plus the actual
parsing logic). These packages are **only imported by `cmd/api/main.go`** — the `ingestion`
package has no idea they exist; it only talks to `provider.All()`. This is the place that
changes when a new provider is added (a new subpackage plus a single line in `main.go`).

### `internal/ingestion`
- `job.go`: `Job`, which runs all registered providers and writes the result to the storage
  layer. It defines its own narrow interfaces (`ContentWriter`, `Indexer`) — it knows those
  interfaces, not the concrete Mongo/ES implementations (dependency inversion). `RunAll` ends
  with `Reconcile()`, which finds records still `indexed=false` and retries indexing them into
  ES (see `architecture_en.md` section 5).
- `scheduler.go`: a scheduler that triggers `Job` periodically (`time.Ticker`-based).

### `internal/storage/mongo`
`Repository`, which encapsulates access to MongoDB. This is the system-of-record layer —
idempotency is guaranteed via upsert keyed on `external_id + provider`. It satisfies the
`ingestion.ContentWriter` interface implicitly (Go has no `implements` keyword).
`FindUnindexed`/`MarkIndexed` are the read/write side of the simplified outbox pattern.

### `internal/storage/elastic`
`Client`, which encapsulates access to Elasticsearch. The search-optimized, rebuildable-from-Mongo
view layer. It satisfies both `ingestion.Indexer` (writing) and `search.Searcher` (reading).
`candidateWindowSize=500` fetches the top 500 BM25-relevant candidates — final ranking is left
to the `scoring` package.

### `internal/scoring`
Pure functions that convert a provider's raw popularity values into a standard score, weighted
by content type. It knows nothing about HTTP, databases, or any external dependency — making it
the easiest package to unit test.

### `internal/search`
The business logic for "what happens when a user searches": fetches candidates from
Elasticsearch, re-scores them via `scoring`, sorts, paginates, and validates the request
(`validateQuery`). It never sees HTTP request/response objects — so the same logic could later
be called from a CLI or a gRPC service.

### `internal/transport/http`
- `router.go`: the "wiring" layer defining which path goes to which handler through which
  middleware chain. Sets up the rate limiter and applies it to `/search` and `/refresh` (not
  `/metrics`).
- `handler/`: a thin layer that converts an HTTP request into domain types and hands it off to
  business logic (`search.go`, `refresh.go`). Handlers contain no business rules.
- `middleware/`: cross-cutting behavior applied to every endpoint — `request_id.go` assigns a
  UUID to every request and carries it through the context (this is what makes searching logs
  by `request_id` in Loki possible), `cors.go` allows a single origin (the dashboard), and
  `ratelimit.go` applies a per-IP token-bucket rate limit.

### `internal/observability/logger`
The single setup point for the structured logger (JSON, stdout, `slog`). If a future migration
to `zap` is needed, this is the only file that changes.

### `internal/observability/metrics`
The single definition point for every custom metric exposed to Prometheus
(`provider_circuit_state`, `provider_requests_total`, `provider_request_duration_seconds`,
`search_request_duration_seconds`, `content_pending_indexing`). Neither the provider nor the
search package defines its own metrics ad hoc — they import from here.

### `mocks/json-provider-a`
Our own third provider — a mix of the schema and concepts from the real JSON and XML
providers, generating a much larger dataset (20,000 records by default). It's a
**separate Go module** (`go.mod`) with its own Docker image — the Normalizer in
`internal/provider/json` can parse this API with the exact same Normalizer used for the real
provider, because the schema is deliberately kept identical.

### `frontend`
The search UI, written in React 18 + TypeScript + Vite, styled with Tailwind CSS and animated
with Framer Motion. `api/client.ts` makes requests to the backend, `components/` holds the UI
pieces — search box, filters, result list/card, score bar, pagination. Built with
`node:20-alpine` and served statically via `nginx:1.27-alpine` in Docker; during development,
Vite's own dev server is used via `npm run dev` (see `local-setup_en.md`).

### `deployments`
All infrastructure configuration outside the Go code: Prometheus scrape/alert rules, the
Alertmanager route, Grafana's auto-provisioned data sources and dashboards, and Loki/Promtail
configuration. These files are mounted read-only into the relevant containers from
`docker-compose.yml`.

## Why it's structured this way

- **Testability**: pure business-logic packages like `scoring` and `search` import no external
  dependencies, so they can be unit tested without mocking anything — in fact,
  `internal/search`, `internal/ingestion`, and `internal/provider` are all covered by tests
  using fake/stub implementations (`*_test.go` files).
- **Isolation of change**: switching away from Mongo would only change
  `internal/storage/mongo`; `ingestion`, `search`, and `scoring` would be untouched, because they
  talk to interfaces, not concrete types.
- **Extensibility**: adding a new provider means two files under `internal/provider/<new>/` plus
  one line in `main.go` — no existing file changes.
- **Readability**: a new team member can look at the folder names and find "where's the
  search logic," "where's the resilience layer" within seconds.
- **Operational clarity**: Go code (`internal/`, `cmd/`) is clearly separated from
  infrastructure configuration (`deployments/`) and external processes (`mocks/`,
  `frontend/`) — the folder structure makes it obvious which change requires rebuilding which
  container.