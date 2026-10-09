#!/usr/bin/env bash
# benchmark.sh — redirect latency: Redis cache HIT vs. cold Postgres MISS
#
# Prerequisites:
#   - The API is running on $BASE (default http://localhost:8080)
#   - `hey` is installed: go install github.com/rakyll/hey@latest
#   - Redis is reachable for the flush step (redis-cli on $REDIS_ADDR)
#   - jq is installed (brew install jq / apt install jq)
#
# Usage:
#   ./scripts/benchmark.sh
#   BASE=http://localhost:8080 REDIS_ADDR=localhost:6379 ./scripts/benchmark.sh
#
# Output is written to docs/benchmark.md (results section only, not the full doc).

set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
REDIS_HOST="${REDIS_ADDR%%:*}"
REDIS_PORT="${REDIS_ADDR##*:}"
EMAIL="bench-$(date +%s)@example.com"
PASSWORD="BenchP@ss123"
REQUESTS=2000
CONCURRENCY=50
REPORT="docs/benchmark_raw.md"

# ── helpers ──────────────────────────────────────────────────────────────────
require() { command -v "$1" >/dev/null 2>&1 || { echo "ERROR: $1 not found. $2"; exit 1; }; }
header() { echo; echo "══════════════════════════════════════════"; echo "  $1"; echo "══════════════════════════════════════════"; }

require hey  "Install with: go install github.com/rakyll/hey@latest"
require jq   "Install with: brew install jq  (macOS) or apt install jq (Linux)"

mkdir -p docs

header "1/5  Registering benchmark user"
curl -sf -X POST "$BASE/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" | jq -r '"  id: " + .id'

header "2/5  Logging in"
TOKEN=$(curl -sf -X POST "$BASE/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" | jq -r .token)
echo "  token obtained"

header "3/5  Creating short link"
CODE=$(curl -sf -X POST "$BASE/api/v1/shorten" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/doc/effective_go"}' | jq -r .code)
echo "  code: $CODE"

REDIRECT_URL="$BASE/$CODE"

# ── Warm the cache with a single request ─────────────────────────────────────
header "4/5  Warming Redis cache"
STATUS=$(curl -so /dev/null -w "%{http_code}" "$REDIRECT_URL")
if [ "$STATUS" != "302" ]; then
  echo "ERROR: expected 302, got $STATUS — is the server running?"
  exit 1
fi
echo "  cache warm (got 302)"

# ── RUN A: cache HIT ──────────────────────────────────────────────────────────
header "5a/5  Benchmarking: Redis cache HIT"
echo "  hey -n $REQUESTS -c $CONCURRENCY -disable-redirects $REDIRECT_URL"
echo
HIT_OUTPUT=$(hey -n "$REQUESTS" -c "$CONCURRENCY" -disable-redirects "$REDIRECT_URL" 2>&1)
echo "$HIT_OUTPUT"

# ── Flush Redis to force Postgres cold path ───────────────────────────────────
header "5b/5  Flushing Redis (FLUSHDB) to force cold path"
if command -v redis-cli >/dev/null 2>&1; then
  redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" FLUSHDB
  echo "  Redis flushed"
else
  echo "  WARNING: redis-cli not found — skipping flush."
  echo "  The MISS run below may still hit the cache."
fi

# ── RUN B: cache MISS (Postgres) ──────────────────────────────────────────────
header "5c/5  Benchmarking: Postgres cold MISS"
echo "  hey -n $REQUESTS -c $CONCURRENCY -disable-redirects $REDIRECT_URL"
echo
MISS_OUTPUT=$(hey -n "$REQUESTS" -c "$CONCURRENCY" -disable-redirects "$REDIRECT_URL" 2>&1)
echo "$MISS_OUTPUT"

# ── Extract p50/p99 summary lines from hey output ────────────────────────────
extract_latency() {
  # hey prints a histogram section; the summary line looks like:
  #   50% in 0.0023 secs
  # We pull the 50th and 99th percentile lines.
  echo "$1" | grep -E "^  (50|99)%" | sed 's/^[[:space:]]*/  /'
}

# ── Write raw report ──────────────────────────────────────────────────────────
{
  echo "# Benchmark raw output"
  echo
  echo "Generated: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "API: $BASE  |  requests: $REQUESTS  |  concurrency: $CONCURRENCY"
  echo
  echo "## Redis cache HIT"
  echo '```'
  echo "$HIT_OUTPUT"
  echo '```'
  echo
  echo "## Postgres cold MISS"
  echo '```'
  echo "$MISS_OUTPUT"
  echo '```'
} > "$REPORT"

echo
echo "══════════════════════════════════════════"
echo "  Summary"
echo "══════════════════════════════════════════"
echo
echo "Redis HIT percentiles:"
extract_latency "$HIT_OUTPUT"
echo
echo "Postgres MISS percentiles:"
extract_latency "$MISS_OUTPUT"
echo
echo "Raw output saved to $REPORT"
echo "Copy the numbers into docs/benchmark.md (see the template there)."
