#!/usr/bin/env bash
# S1-06 staging authorization check for the audited routes (R1–R11).
#
# Runs against a disposable/staging gateway with synthetic data only.
# Sends requests that may create or delete data only when the caller is
# expected to be refused (403); allowed-role requests use non-existent IDs or
# invalid bodies, so they must not change real data. Review before running.
#
# Required environment (token values are never printed):
#   OBSERVEX_URL          e.g. https://observex-staging.example.com
#   VIEWER_JWT            JWT of a viewer in org A
#   EDITOR_JWT            JWT of an editor in org A
#   ADMIN_JWT             JWT of an org admin in org A
# Optional:
#   CURL_OPTS             extra curl options (e.g. --cacert ca.pem)
#
# Exit code: 0 when every check matches, 1 otherwise.

set -euo pipefail

: "${OBSERVEX_URL:?set OBSERVEX_URL}"
: "${VIEWER_JWT:?set VIEWER_JWT}"
: "${EDITOR_JWT:?set EDITOR_JWT}"
: "${ADMIN_JWT:?set ADMIN_JWT}"

base="${OBSERVEX_URL%/}"
fail=0
missing_id="s106-nonexistent-$(date +%s)"

# status METHOD PATH ROLE [BODY]  -> prints HTTP status only
status() {
  local method=$1 path=$2 role=$3 body=${4:-} token=""
  case "$role" in
    none)   token="" ;;
    viewer) token=$VIEWER_JWT ;;
    editor) token=$EDITOR_JWT ;;
    admin)  token=$ADMIN_JWT ;;
    invalid) token="not-a-valid-jwt" ;;
  esac
  local args=(-s -o /dev/null -w '%{http_code}' -X "$method" --config -)
  # shellcheck disable=SC2206
  [[ -n "${CURL_OPTS:-}" ]] && args+=(${CURL_OPTS})
  [[ -n "$body" ]] && args+=(-H 'Content-Type: application/json' --data "$body")
  # The Authorization header is passed on stdin (curl --config -) so the token
  # does not appear in the process list or in this script's output.
  if [[ -n "$token" ]]; then
    printf 'header = "Authorization: Bearer %s"\n' "$token" | curl "${args[@]}" "${base}${path}"
  else
    printf '' | curl "${args[@]}" "${base}${path}"
  fi
}

# expect ID METHOD PATH ROLE WANT [BODY]
expect() {
  local id=$1 method=$2 path=$3 role=$4 want=$5 body=${6:-}
  local got
  got=$(status "$method" "$path" "$role" "$body")
  if [[ "$got" == "$want" ]]; then
    printf 'PASS  %-4s %-6s %-45s %-7s -> %s\n' "$id" "$method" "$path" "$role" "$got"
  else
    printf 'FAIL  %-4s %-6s %-45s %-7s -> %s (want %s)\n' "$id" "$method" "$path" "$role" "$got" "$want"
    fail=1
  fi
}

# expect_not ID METHOD PATH ROLE NOT_A NOT_B [BODY]  (role check must pass)
expect_not() {
  local id=$1 method=$2 path=$3 role=$4 not_a=$5 not_b=$6 body=${7:-}
  local got
  got=$(status "$method" "$path" "$role" "$body")
  if [[ "$got" != "$not_a" && "$got" != "$not_b" ]]; then
    printf 'PASS  %-4s %-6s %-45s %-7s -> %s (not %s/%s)\n' "$id" "$method" "$path" "$role" "$got" "$not_a" "$not_b"
  else
    printf 'FAIL  %-4s %-6s %-45s %-7s -> %s (must not be %s/%s)\n' "$id" "$method" "$path" "$role" "$got" "$not_a" "$not_b"
    fail=1
  fi
}

echo "S1-06 authorization checks against ${base} ($(date -u +%FT%TZ))"

routes=(
  "R1|POST|/api/v1/postmortems|editor"
  "R2|PUT|/api/v1/postmortems/${missing_id}|editor"
  "R3|DELETE|/api/v1/postmortems/${missing_id}|editor"
  "R4|POST|/api/v1/integrations|admin"
  "R5|DELETE|/api/v1/integrations/${missing_id}|admin"
  "R6|POST|/api/v1/integrations/${missing_id}/test|admin"
  "R7|DELETE|/api/v1/problems/${missing_id}/comments/${missing_id}|editor"
  "R8|POST|/api/v1/llm/ingest|editor"
  "R9|GET|/api/v1/remediations|editor"
  "R10|GET|/api/v1/agents|editor"
)

for entry in "${routes[@]}"; do
  IFS='|' read -r id method path min <<<"$entry"
  expect "$id" "$method" "$path" none 401
  expect "$id" "$method" "$path" invalid 401
  expect "$id" "$method" "$path" viewer 403
  if [[ "$min" == admin ]]; then
    expect "$id" "$method" "$path" editor 403
  fi
done

# Allowed roles pass the role check. Invalid bodies / unknown IDs keep data unchanged.
expect_not R1 POST   /api/v1/postmortems editor 401 403 '{}'
expect_not R4 POST   /api/v1/integrations admin 401 403 '{}'
expect     R7 DELETE "/api/v1/problems/${missing_id}/comments/${missing_id}" editor 404
expect     R9 GET    /api/v1/remediations editor 403
expect     R9 GET    /api/v1/remediations admin 403
expect_not R10 GET   /api/v1/agents editor 401 403

# R11 WebSocket: plain GET (no upgrade headers)
expect R11 GET /ws none 401
expect R11 GET /ws invalid 401
expect R11 GET /ws editor 426
expect R11 GET "/ws?token=${missing_id}" none 401

if [[ $fail -ne 0 ]]; then
  echo "RESULT: FAIL"
  exit 1
fi
echo "RESULT: PASS (authorization status codes only; not a full validation)"
