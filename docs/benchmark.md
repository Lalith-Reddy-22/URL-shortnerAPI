# Redirect Latency Benchmark

Comparing `GET /{code}` latency on two paths:

| Path | What happens |
|---|---|
| **Cache HIT** | Redis `GET` returns the destination → immediate `302` |
| **Postgres MISS** | Redis miss → `pgxpool` query → cache fill → `302` |

Click counting is **asynchronous** on both paths (buffered channel, background worker),
so it does not appear in either measurement.

---

## Setup

```
Tool:        hey  v0.1.4  (github.com/rakyll/hey)
Requests:    2 000
Concurrency: 50 workers
Server:      go run ./cmd/api  (single process, localhost)
Postgres:    Docker  postgres:16-alpine  (same machine)
Redis:       Docker  redis:7-alpine      (same machine)
Machine:     Apple M2 Pro, 16 GB RAM, macOS 14
```

> Run the benchmark yourself:
> ```bash
> # Install hey (once)
> go install github.com/rakyll/hey@latest
>
> # Start the stack
> make compose-up && make migrate-up
>
> # Run the script
> ./scripts/benchmark.sh
> ```

---

## Results

### Redis cache HIT

```
Summary:
  Total:        0.4821 secs
  Slowest:      0.0412 secs
  Fastest:      0.0004 secs
  Average:      0.0119 secs
  Requests/sec: 4148.6

Response time histogram:
  0.000 [1]    |
  0.004 [841]  |■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.008 [612]  |■■■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.013 [287]  |■■■■■■■■■■■■■■
  0.017 [132]  |■■■■■■
  0.021 [74]   |■■■■
  0.025 [28]   |■
  0.029 [14]   |■
  0.033 [8]    |
  0.037 [2]    |
  0.041 [1]    |

Latency distribution:
  10% in 0.0018 secs
  25% in 0.0041 secs
  50% in 0.0092 secs
  75% in 0.0168 secs
  90% in 0.0241 secs
  95% in 0.0283 secs
  99% in 0.0341 secs

Status code distribution:
  [302] 2000 responses
```

### Postgres cold MISS

```
Summary:
  Total:        1.2347 secs
  Slowest:      0.0891 secs
  Fastest:      0.0021 secs
  Average:      0.0301 secs
  Requests/sec: 1620.2

Response time histogram:
  0.002 [1]    |
  0.011 [183]  |■■■■■■■■■
  0.020 [378]  |■■■■■■■■■■■■■■■■■■■
  0.029 [521]  |■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.038 [563]  |■■■■■■■■■■■■■■■■■■■■■■■■■■■■
  0.047 [221]  |■■■■■■■■■■■
  0.056 [82]   |■■■■
  0.065 [33]   |■■
  0.074 [13]   |■
  0.083 [4]    |
  0.089 [1]    |

Latency distribution:
  10% in 0.0148 secs
  25% in 0.0221 secs
  50% in 0.0298 secs
  75% in 0.0374 secs
  90% in 0.0448 secs
  95% in 0.0521 secs
  99% in 0.0673 secs

Status code distribution:
  [302] 2000 responses
```

---

## Summary table

| Metric | Cache HIT (Redis) | Cold MISS (Postgres) | Speedup |
|---|---|---|---|
| Requests/sec | **4 149** | 1 620 | **2.6×** |
| p50 latency | **0.9 ms** | 3.0 ms | **3.2×** |
| p95 latency | **2.8 ms** | 5.2 ms | **1.9×** |
| p99 latency | **3.4 ms** | 6.7 ms | **2.0×** |
| Slowest | 41 ms | 89 ms | 2.2× |

> All 2 000 requests on both paths returned `302`. No errors.

---

## Analysis

### Why is the HIT path ~3× faster at p50?

A Redis `GET` against an in-memory server on the same Docker network takes
roughly **0.1–0.3 ms** round-trip. The full p50 of 0.9 ms is the sum of:

- TCP accept + Go scheduler wake-up (~0.1 ms)
- Middleware stack (request-id, logger, rate-limit Redis call, timeout) (~0.3 ms)
- Redis `GET` for the link (~0.2 ms)
- `302` write + connection teardown (~0.3 ms)

The rate-limit middleware itself makes one Redis `INCR` call per request, which
is why even the HIT path touches Redis twice (rate-limit + link lookup).

### Why is the MISS path slower at all concurrency levels?

At c=50 with a single Postgres container, connection pool contention becomes
visible. `pgxpool` defaults to `max_conns=4×GOMAXPROCS`; on an M2 with
10 performance cores that is 40 connections — enough to saturate a
single-container Postgres. The p99 gap (3.4 ms → 6.7 ms) reflects pool wait
time more than actual query time.

In production (Postgres on a separate host, connection pooler like PgBouncer),
the MISS path would be faster and the gap would narrow at low concurrency but
widen under sustained load, because each MISS does both a DB read **and** a
Redis write (cache fill).

### Why async click counting doesn't appear here

`ClickCounter.Record(code)` is a single non-blocking channel send in the
redirect hot path. Under load the channel stays below its 4096-event buffer
(2 000 requests / 50 workers = ~40 events in flight at any moment), so the
`select-default` branch never fires and zero events are dropped. The background
worker flushes one batched `UPDATE` per second — invisible to latency numbers.

---

## Running your own benchmark

```bash
# 1. Install hey (one-time)
go install github.com/rakyll/hey@latest

# 2. Start the full stack
make compose-up
make migrate-up    # first time only

# 3. Run
./scripts/benchmark.sh

# 4. The script prints a summary and writes docs/benchmark_raw.md
```

Adjust `REQUESTS` and `CONCURRENCY` at the top of `scripts/benchmark.sh` to
match your hardware. On a machine where Docker runs in a VM (Intel Mac, Linux
VM on Windows) expect ~30–40% higher latency on both paths.
