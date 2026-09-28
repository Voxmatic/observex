#!/usr/bin/env bash
# S1-04 staging check: the query engine exposes no raw SQL execution path.
#
# The query engine is an internal service, so run this from inside the
# cluster / Compose network (for example from a debug pod), against a
# disposable or staging deployment. It sends no credentials.
#
# Safety: every SQL probe is a harmless read-only literal (SELECT 1 / SELECT
# 's1-04-probe'), so even an unpatched query engine would not change data.
# Never add destructive statements to this script.
#
# Required environment:
#   QUERY_ENGINE_URL   e.g. http://query-engine:9090
# Optional:
#   CURL_OPTS          extra curl options (e.g. --cacert ca.pem)
#
# Exit code: 0 when every check matches, 1 otherwise. A connection error
# stops the script with curl's non-zero exit code.

set -euo pipefail

: "${QUERY_ENGINE_URL:?set QUERY_ENGINE_URL}"

base="${QUERY_ENGINE_URL%/}"
fail=0

# status METHOD PATH [BODY]  -> prints HTTP status only
status() {
  local method=$1 path=$2 body=${3:-}
  local args=(-s -o /dev/null -w '%{http_code}' -X "$method" --max-time 20)
  # shellcheck disable=SC2206
  [[ -n "${CURL_OPTS:-}" ]] && args+=(${CURL_OPTS})
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' --data "$body")
  curl "${args[@]}" "${base}${path}"
}

# expect ID METHOD PATH WANT [BODY]
expect() {
  local id=$1 method=$2 path=$3 want=$4 body=${5:-}
  local got
  got=$(status "$method" "$path" "$body")
  if [[ "$got" == "$want" ]]; then
    printf 'PASS  %-4s %-6s %-28s -> %s\n' "$id" "$method" "$path" "$got"
  else
    printf 'FAIL  %-4s %-6s %-28s -> %s (want %s)\n' "$id" "$method" "$path" "$got" "$want"
    fail=1
  fi
}

# expect_not ID METHOD PATH NOT_A NOT_B [BODY]
expect_not() {
  local id=$1 method=$2 path=$3 not_a=$4 not_b=$5 body=${6:-}
  local got
  got=$(status "$method" "$path" "$body")
  if [[ "$got" != "$not_a" && "$got" != "$not_b" && "$got" != "000" ]]; then
    printf 'PASS  %-4s %-6s %-28s -> %s (not %s/%s)\n' "$id" "$method" "$path" "$got" "$not_a" "$not_b"
  else
    printf 'FAIL  %-4s %-6s %-28s -> %s (must not be %s/%s/000)\n' "$id" "$method" "$path" "$got" "$not_a" "$not_b"
    fail=1
  fi
}

echo "S1-04 raw SQL removal checks against ${base} ($(date -u +%FT%TZ))"

# Q0: service reachable
expect Q0 GET /health 200

# Q1: /query/events is gone for every method, with and without an sql field
for method in POST GET PUT DELETE PATCH; do
  expect Q1 "$method" /query/events 404
  expect Q1 "$method" /query/events 404 '{"sql":"SELECT 1"}'
done
expect Q1 POST /query/events 404 '{}'
expect Q1 POST /query/events 404 '{"limit":50}'

# Q2: POST /query no longer accepts the former sql/events types
for typ in sql events SQL Events; do
  expect Q2 POST /query 400 "{\"type\":\"${typ}\",\"query\":\"SELECT 's1-04-probe'\"}"
done

# Q3: control — unknown types were already rejected
expect Q3 POST /query 400 '{"type":"s1-04-unknown","query":"x"}'

# Q4: native metric reads still routed (200, or 502 if the metric store is down)
expect_not Q4 GET "/query/instant?query=up" 404 405
expect_not Q4 GET "/api/v1/query?query=up" 404 405

if [[ $fail -ne 0 ]]; then
  echo "RESULT: FAIL"
  exit 1
fi
echo "RESULT: PASS (status codes only; not a full validation)"
