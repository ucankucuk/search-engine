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
├── go.mod
├── README.md
├── cmd/
│   └── api/
│       └── main.go
└── internal/
    ├── domain/
    │   ├── content.go
    │   └── errors.go
    ├── config/
    │   └── config.go
    ├── provider/
    │   ├── provider.go
    │   ├── fetcher.go
    │   ├── adapter.go
    │   ├── resilient.go
    │   ├── json/
    │   │   ├── fetcher.go
    │   │   └── normalizer.go
    │   └── xml/
    │       ├── fetcher.go
    │       └── normalizer.go
    ├── ingestion/
    │   ├── job.go
    │   └── scheduler.go
    ├── storage/
    │   ├── mongo/
    │   │   └── repository.go
    │   └── elastic/
    │       └── index.go
    ├── scoring/
    │   └── scoring.go
    ├── search/
    │   └── service.go
    ├── transport/
    │   └── http/
    │       ├── router.go
    │       ├── handler/
    │       │   ├── search.go
    │       │   └── refresh.go
    │       └── middleware/
    │           └── request_id.go
    └── observability/
        ├── logger/
        │   └── logger.go
        └── metrics/
            └── metrics.go
```

## What each package is for

### `cmd/api`
**main.go** — the application's single entry point (composition root). Reads config, opens the
Mongo/Elasticsearch connections, registers providers, applies the resilience layer, wires up
services and HTTP handlers, and starts the server. **It contains no business rules** — only
information about which piece connects to which.

### `internal/domain`
Holds the application's core data models (`Content`, `ScoredContent`, `SearchQuery`) and shared
error types (`ErrContentNotFound`, etc.). **It imports nothing** — this is the Go equivalent of
the "entities" layer in clean architecture. Every other package can import it; it imports none
of them. It has no idea whether a provider speaks JSON or XML, or whether data lives in Mongo or
Elasticsearch.

### `internal/config`
Reads environment variables into a type-safe `Config` struct. No other package in the project
calls `os.Getenv` directly — this is the single place that answers "what environment variables
exist."

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
- `job.go`: `Job`, which runs all registered providers (or a single one) and writes the result
  to the storage layer. It defines its own narrow interfaces (`ContentWriter`, `Indexer`) —
  it knows those interfaces, not the concrete Mongo/ES implementations (dependency inversion).
- `scheduler.go`: a scheduler that triggers `Job` periodically (`time.Ticker`-based; can be
  swapped for `robfig/cron` if needed).

### `internal/storage/mongo`
`Repository`, which encapsulates access to MongoDB. This is the system-of-record layer —
idempotency is guaranteed via upsert keyed on `external_id`. It satisfies the
`ingestion.ContentWriter` interface implicitly (Go has no `implements` keyword).

### `internal/storage/elastic`
`Client`, which encapsulates access to Elasticsearch. The search-optimized, rebuildable-from-Mongo
view layer. It satisfies both `ingestion.Indexer` (writing) and `search.Searcher` (reading).

### `internal/scoring`
Pure functions that convert a provider's raw popularity values into a standard score, weighted
by content type. It knows nothing about HTTP, databases, or any external dependency — making it
the easiest package to unit test.

### `internal/search`
The business logic for "what happens when a user searches": fetches candidates from
Elasticsearch, re-scores them via `scoring`, sorts, paginates. It never sees HTTP
request/response objects — so the same logic could later be called from a CLI or a gRPC service.

### `internal/transport/http`
- `router.go`: the "wiring" layer defining which path goes to which handler through which
  middleware chain.
- `handler/`: a thin layer that converts an HTTP request into domain types and hands it off to
  business logic (`search.go`, `refresh.go`). Handlers contain no business rules.
- `middleware/`: cross-cutting behavior applied to every endpoint — `request_id.go` assigns a
  UUID to every request and carries it through the context (this is what makes searching logs by
  `request_id` in Loki possible), and logs each request.

### `internal/observability/logger`
The single setup point for the structured logger (JSON, stdout). If a future migration to `zap`
is needed, this is the only file that changes.

### `internal/observability/metrics`
The single definition point for every custom metric exposed to Prometheus
(`provider_circuit_state`, `provider_requests_total`, `provider_request_duration_seconds`,
`search_request_duration_seconds`). Neither the provider nor the search package defines its own
metrics ad hoc — they import from here.

## Why it's structured this way

- **Testability**: pure business-logic packages like `scoring` and `search` import no external
  dependencies, so they can be unit tested without mocking anything.
- **Isolation of change**: switching away from Mongo would only change
  `internal/storage/mongo`; `ingestion`, `search`, and `scoring` would be untouched, because they
  talk to interfaces, not concrete types.
- **Extensibility**: adding a new provider means two files under `internal/provider/<new>/` plus
  one line in `main.go` — no existing file changes.
- **Readability**: an interviewer or a new team member can look at the folder names and find
  "where's the search logic," "where's the resilience layer" within seconds — which directly
  satisfies the "clean and understandable code structure" expectation from the case study brief.
