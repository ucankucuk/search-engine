# Local Setup and Running

This document describes the current, verified way to run the project locally with Docker
Compose. For design decisions see `architecture_en.md`; for package layout see
`package-structure_en.md`.

## Prerequisites

| Tool | What it's for | Check command |
|---|---|---|
| Docker + Docker Compose | Running the whole system (Mongo, ES, Prometheus, Grafana, Loki, mock provider, frontend...) | `docker --version` / `docker compose version` |
| Go (1.24+) | Only needed if you want to build/run the backend locally without Docker | `go version` |
| Node.js (18+) + npm | Only needed if you want to iterate on the dashboard without Docker | `node --version` |

Docker alone is enough for day-to-day use — Go and Node are only needed when developing that
layer separately.

## Quick start (Docker Compose — recommended)

```bash
cp .env.example .env      # adjust provider URLs if needed
go mod tidy                # generates go.sum — the Dockerfile expects this, REQUIRED
docker compose up -d --build
```

The startup order is deliberately layered: MongoDB + Elasticsearch must pass their
healthchecks first, only then does `app` start; once `app` is healthy, Prometheus/Loki/
Promtail/Grafana/Alertmanager start (see the comments in `docker-compose.yml` for detail).

Once everything is up:

| Service | Address |
|---|---|
| API | http://localhost:8080 |
| MongoDB | mongodb://localhost:27017 |
| Mongo Express (browse Mongo data) | http://localhost:8081 (admin/admin) |
| Elasticsearch | http://localhost:9200 |
| Elasticvue (browse ES data) | http://localhost:8082 |
| Prometheus | http://localhost:9090 |
| Alertmanager | http://localhost:9093 |
| Grafana | http://localhost:3000 (admin/admin) |
| Loki | http://localhost:3100 (used via Grafana → Explore) |
| json-provider-a (our own mock) | http://localhost:4001/json-a |
| Dashboard (frontend) | http://localhost:5173 |

**Verify:**
```bash
docker compose ps                 # everything should be "healthy" or "running"
docker compose logs app --tail=50 # look for the "sunucu başlatılıyor" (server starting) line
curl "http://localhost:8080/search?q=go&page=1&page_size=5"
```

## Rebuilding after a code change

If your change only affects the `app` service:

```bash
docker compose build --no-cache app
docker compose up -d
```

**Don't skip `--no-cache`** — Docker sometimes assumes "nothing changed" and reuses a cached
layer that still contains the old binary. In a workflow with frequent file changes, this can
make a container look normal in `docker compose ps` while actually running stale code.

To rebuild/restart only specific services, name them explicitly:
```bash
docker compose build --no-cache prometheus alertmanager
docker compose up -d prometheus alertmanager
```

## Running the tests

From the project root:
```bash
go test ./... -v
```
- `./...` discovers tests in every subpackage (`internal/search`, `internal/ingestion`,
  `internal/provider`).
- `-v` shows a PASS/FAIL line for every individual test.

Expected: all three packages report `ok`, with no `FAIL`. In GoLand, you can run the same
tests by clicking the ▶ icon next to a test function, or by right-clicking the `internal`
folder and choosing "Run 'go test ./...'".

## Manual outage/resilience testing

There's deliberately no scheduled automatic outage simulation in `docker-compose.yml` —
provider outages are triggered manually, so you can test them on demand, predictably:

```bash
docker compose stop json-provider-a     # take the provider down
docker compose logs app -f              # watch the breaker's closed→open→half-open transitions
docker compose start json-provider-a    # bring it back up
```

Expected flow: after a few consecutive failures (~5-10s) the breaker trips to **Open**; once
the provider is back, it should return to **Closed** via **HalfOpen** within roughly 10-20
seconds. At the same time you can watch `ProviderCircuitBreakerOpen` go Firing→Resolved at
`http://localhost:9090/alerts`, and see the alert land in Alertmanager at
`http://localhost:9093`.

To test the Mongo↔ES consistency mechanism (indexed flag + reconcile), use the same approach
with Elasticsearch: `docker compose stop elasticsearch` / `start elasticsearch` — you should
see `content_pending_indexing` climb while ES is down and drop back to zero once it recovers.

## Developing the dashboard without Docker (fast iteration)

The `frontend` service in Docker requires a rebuild on every change — for daily development,
Vite's own dev server is much faster:

```bash
cd frontend
npm install
cp .env.example .env      # VITE_API_URL already points at localhost:8080
npm run dev
```

Runs at `http://localhost:5173` with live hot reload. The backend (`go run ./cmd/api`, or the
`app` service from Docker Compose) needs to be running as well.

## Common pitfalls

- **`app` container shows `unhealthy` / logs look wrong**: check the contents of
  `cmd/api/main.go` — since both `mocks/json-provider-a/main.go` and `cmd/api/main.go` share
  the filename `main.go`, it's easy to accidentally save the wrong content to the wrong path.
  The first line of `cmd/api/main.go` should be a comment describing it as the app's single
  entry point; if it instead describes a mock JSON provider, the files got swapped.
- **A code change doesn't seem to take effect**: do a clean rebuild with
  `docker compose build --no-cache <service>` (see above).
- **No container-level CPU/RAM metrics from cAdvisor**: this is intentional — it was removed
  entirely because it didn't work reliably on Docker Desktop for Mac's LinuxKit VM (a
  mount-propagation limitation). Only Go runtime metrics (heap, goroutines, CPU) are kept; see
  `architecture_en.md` section 10.
- **The Go version in `go.mod` doesn't match your machine**: `go.mod` specifies `go 1.27`; a
  newer local Go toolchain works fine, an older one will need an upgrade.