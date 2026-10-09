# URL Shortener

A production-style REST API that shortens URLs — built in Go as a portfolio
project demonstrating layered architecture, Redis cache-aside, async click
counting, JWT auth, and full CI/CD.

---

## Architecture

```mermaid
flowchart TD
    Client -->|HTTP| Router["chi Router\n(RequestID · Logger · Recoverer\nTimeout · RateLimit · JWTAuth)"]

    Router -->|POST /api/v1/shorten| ShortenHandler["handler.Links.Shorten"]
    Router -->|GET /{code}| RedirectHandler["handler.Links.Redirect"]
    Router -->|POST /api/v1/auth/*| AuthHandler["handler.Auth"]
    Router -->|GET /api/v1/links*| LinksHandler["handler.Links (Stats/List/Delete)"]

    ShortenHandler --> LinkService["service.Links"]
    RedirectHandler --> LinkService
    LinksHandler --> LinkService
    AuthHandler --> AuthService["service.Auth"]

    LinkService -->|read/write| Postgres[("PostgreSQL\n(links, users, clicks)")]
    LinkService -->|cache-aside| Redis[("Redis\n(code → CachedLink)")]
    LinkService -->|non-blocking| ClickCh["buffered channel\n(4096)"]
    ClickCh --> Worker["ClickCounter worker\n(batches of 100)"]
    Worker -->|AddClicks| Postgres

    AuthService --> Postgres
```

### Request lifecycle for `GET /{code}`

```
Client
  │
  ▼
Middleware stack (request-id, log, recover, timeout, rate-limit)
  │
  ▼
handler.Redirect
  │
  ├─ Redis HIT? ──► check expiry ──► 302 redirect
  │                                     │
  │                              Record(code) ──► buffered channel (never blocks)
  │
  └─ Redis MISS
       │
       ▼
     Postgres query
       │
       ├─ not found ──► 404
       ├─ expired   ──► 410
       └─ found ──► SetLink (Redis, TTL=min(cache_ttl, remaining_expiry))
                      │
                      ▼
                  302 redirect + Record(code)
```

---

## Stack

| Layer | Choice |
|---|---|
| Language | Go 1.22+ |
| Router | [go-chi/chi v5](https://github.com/go-chi/chi) |
| Database | PostgreSQL 16 via [pgx v5](https://github.com/jackc/pgx) (pgxpool) |
| Cache | Redis 7 via [go-redis v9](https://github.com/redis/go-redis) |
| Auth | [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt) + bcrypt |
| Logging | `log/slog` (structured JSON) |
| Migrations | plain SQL files + [golang-migrate](https://github.com/golang-migrate/migrate) |
| Container | Docker (distroless/static final image) |
| CI/CD | GitHub Actions → GHCR |

---

## Project layout

```
cmd/api/
  main.go          # wiring: config, pools, services, graceful shutdown
  router.go        # chi router + middleware stack

internal/
  config/          # env-var config with validation
  handler/         # HTTP handlers (thin: decode → service → encode)
  middleware/      # RequestID, Logger, Recoverer, Timeout, RateLimit, JWTAuth
  model/           # plain structs (Link, User, CachedLink, ClickDelta)
  repository/      # Postgres + Redis behind Store interfaces
  service/         # business logic (shorten, resolve, auth, click counting)

migrations/        # SQL up/down migrations (no ORM)

.github/workflows/
  ci.yml           # lint → test (with real PG+Redis) → build+push on main

Dockerfile         # multi-stage: builder (golang:alpine) + runtime (distroless)
docker-compose.yml # postgres + redis + api
openapi.yaml       # OpenAPI 3.1 spec
```

---

## Setup

### Prerequisites

- Docker + Docker Compose (for `make compose-up`)
- Go 1.22+ (for `make run` on the host)
- `golang-migrate` (fetched automatically by the `make migrate-*` targets via `go run`)

### 1. Copy env config

```bash
cp .env.example .env
# Edit .env and set a real JWT_SECRET before any deployment.
```

### 2. Start dependencies (Postgres + Redis)

```bash
make compose-up
```

The `docker-compose.yml` starts the API container as well. Skip to step 4 if
you want the fully-containerised setup.

### 3. Run the API on the host (dev mode)

```bash
make migrate-up   # apply migrations once
make run          # go run ./cmd/api
```

### 4. Fully containerised

```bash
docker compose up --build
# In a second terminal, run migrations against the containerised postgres:
DATABASE_URL=postgres://urlshortener:urlshortener@localhost:5432/urlshortener?sslmode=disable \
  make migrate-up
```

### 5. Verify

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}

curl http://localhost:8080/readyz
# {"postgres":"ok","redis":"ok"}
```

---

## API quick reference

Full spec: [`openapi.yaml`](./openapi.yaml)

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | — | Register |
| `POST` | `/api/v1/auth/login` | — | Login → JWT |
| `POST` | `/api/v1/shorten` | JWT | Create short link |
| `GET` | `/{code}` | — | Redirect (302 / 404 / 410) |
| `GET` | `/api/v1/links` | JWT | List my links (paginated) |
| `GET` | `/api/v1/links/{code}/stats` | JWT owner | Click stats |
| `DELETE` | `/api/v1/links/{code}` | JWT owner | Delete link |
| `GET` | `/healthz` | — | Liveness |
| `GET` | `/readyz` | — | Readiness |

---

## curl examples

All examples assume `BASE=http://localhost:8080`.

### Register & login

```bash
# Register
curl -s -X POST $BASE/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"s3cr3tP@ss"}' | jq

# Login → save the token
TOKEN=$(curl -s -X POST $BASE/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"s3cr3tP@ss"}' | jq -r .token)
echo $TOKEN
```

### Shorten a URL

```bash
# Auto-generated code
curl -s -X POST $BASE/api/v1/shorten \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/doc/effective_go"}' | jq
# {"code":"aB3cD4e","original_url":"https://go.dev/doc/effective_go","expires_at":null}

# Custom alias with expiry
curl -s -X POST $BASE/api/v1/shorten \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev","custom_alias":"godev","expires_at":"2025-12-31T23:59:59Z"}' | jq
```

### Redirect

```bash
# Follow the redirect (-L) or inspect it without following
curl -v $BASE/aB3cD4e
curl -sI $BASE/aB3cD4e | grep -i location
```

### Stats, list, delete

```bash
# Stats
curl -s $BASE/api/v1/links/aB3cD4e/stats \
  -H "Authorization: Bearer $TOKEN" | jq

# List (page 1, 10 per page)
curl -s "$BASE/api/v1/links?page=1&page_size=10" \
  -H "Authorization: Bearer $TOKEN" | jq

# Delete
curl -s -X DELETE $BASE/api/v1/links/aB3cD4e \
  -H "Authorization: Bearer $TOKEN" -o /dev/null -w "%{http_code}"
# 204
```

---

## Running tests

```bash
# Unit + integration (all packages)
make test

# With race detector and per-package coverage
make test-race

# Show coverage in the browser
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

Current coverage (no live DB/Redis needed — mocks everywhere):

| Package | Coverage |
|---|---|
| `cmd/api` | 26 % (wiring; tested via integration) |
| `internal/config` | 91 % |
| `internal/handler` | 90 % |
| `internal/middleware` | 87 % |
| `internal/model` | 100 % |
| `internal/repository` | 89 % |
| `internal/service` | 85 % |

---

## Design decisions

### Why cache-aside (not write-through)?

**Write-through** would populate the cache on every `POST /shorten`. But links
can have expiry times that may be far in the future, and most freshly-created
links are never clicked more than once. Caching everything eagerly wastes
Redis memory. Cache-aside only promotes a link to Redis on first redirect, so
the cache reflects actual hot links automatically and TTL is bounded by the
link's own expiry.

### Why async click counting?

The redirect path is latency-critical. Waiting for a Postgres `UPDATE` on each
`GET /{code}` would add ~1–5 ms per request and would serialize under high
concurrency. Instead, `ClickCounter.Record(code)` sends on a buffered channel
(size 4096) in a non-blocking `select`-default: the handler returns the `302`
immediately. A single background goroutine drains the channel in batches
(default: 100 events or 1-second ticker, whichever comes first) and writes one
`UPDATE` per distinct code. At 1,000 RPS and 100% hit rate this reduces
Postgres click writes by ~100×.

### Why 7-char base62 for short codes?

62^7 = ~3.5 trillion combinations. With a uniform random generator, the expected
number of codes to generate before a collision (birthday bound) is ~1.7 million.
That is more than enough for a portfolio project and most real-world deployments.
Collisions are handled by retrying up to 5 times rather than a pre-check query,
because a pre-check introduces a TOCTOU race against concurrent inserts.

### Why no ORM?

ORMs hide SQL, which makes query tuning hard and produces N+1 queries by
default. Plain SQL with `pgx` is explicit, easy to read in code review, and
lets the interviewer see exactly what hits the database.

### Why `distroless/static` for the Docker image?

The final image contains only the statically-compiled binary and a CA bundle.
No shell, no package manager, no `apt` — the attack surface is minimal, and
`docker scout` (or Trivy) will report near-zero CVEs. The trade-off is that
`docker exec` doesn't work for debugging; use `dlv` remotely or a debug build
on a separate tag.

---

## CI/CD

On every **pull request**:
1. `gofmt` check (fails if any file needs reformatting)
2. `go vet`
3. `golangci-lint` (errcheck, staticcheck, gosec, misspell, revive, …)
4. `go test -race -coverprofile` against real Postgres 16 + Redis 7 sidecars

On **push to main** (after lint + test pass):
5. Multi-stage Docker build
6. Push to GitHub Container Registry (`ghcr.io/<owner>/url-shortener:latest` + `sha-<commit>`)

```bash
# Pull the latest image
docker pull ghcr.io/<your-github-username>/url-shortener:latest
```

---

## Benchmark

See [`docs/benchmark.md`](./docs/benchmark.md) for redirect latency numbers comparing Redis cache HIT vs. cold Postgres MISS (run with `hey`, 2 000 requests, 50 concurrent workers).

Quick numbers from an Apple M2 Pro (localhost Docker):

| Metric | Cache HIT (Redis) | Cold MISS (Postgres) |
|---|---|---|
| Requests/sec | **4 149** | 1 620 |
| p50 latency | **0.9 ms** | 3.0 ms |
| p99 latency | **3.4 ms** | 6.7 ms |

Run it yourself:
```bash
go install github.com/rakyll/hey@latest   # one-time
make benchmark
```
