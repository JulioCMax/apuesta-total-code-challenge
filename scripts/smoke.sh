#!/usr/bin/env bash
# Smoke test: login -> calculate -> place -> balance, against a running
# instance of the API (design.md's Testing Strategy "Smoke" row; used by
# both docker-compose (Phase 13) and scripts/deploy-aws.sh (Phase 15)).
#
# Usage: scripts/smoke.sh [base_url]
#   base_url defaults to http://localhost:8080 (docker-compose's local port).
#
# No jq dependency on purpose: this script must run in a bare Git Bash /
# minimal shell environment with no extra tooling installed.
set -euo pipefail

BASE_URL="${1:-http://localhost:8080}"
DEMO_EMAIL="demo1@apuestatotal.com"
DEMO_PASSWORD="Demo1234!"
# México vs Sudáfrica's 1X2 market, "México" selection — present in the
# seed dataset regardless of environment (docker-compose or a live AWS
# deployment, since both load the same embedded data.json).
SELECTION_ID="0ML784926076341366984H"

log() { printf '[smoke] %s\n' "$1"; }
fail() {
  printf '[smoke] FAILED: %s\n' "$1" >&2
  exit 1
}

# extract_field pulls a top-level "key":"value" or "key":value pair out of
# a compact JSON body without a jq dependency. Sufficient for this script's
# own well-known, self-controlled response shapes.
extract_field() {
  local body="$1" key="$2"
  printf '%s' "$body" | grep -o "\"${key}\":\"\?[^,}\"]*\"\?" | head -n1 | sed -E "s/\"${key}\":\"?([^,}\"]*)\"?/\1/"
}

# call_api issues one curl request and sets $LAST_STATUS / $LAST_BODY from
# the response, so every step below can assert the literal HTTP status
# instead of only pattern-matching a substring out of the body. Without
# this, a REJECTED placement's 409 error envelope (which embeds
# "status":"rejected" inside error.details) was indistinguishable from a
# successful 201 to a script that never looked at the status code at all.
call_api() {
  local raw
  raw=$(curl -sS -w '\n%{http_code}' "$@")
  LAST_STATUS="${raw##*$'\n'}"
  LAST_BODY="${raw%$'\n'*}"
}

# expect_status fails loudly when the last call_api response did not carry
# exactly the expected HTTP status — the whole point of this rewrite.
expect_status() {
  local step="$1" expected="$2"
  [ "$LAST_STATUS" = "$expected" ] || fail "${step}: expected HTTP ${expected}, got ${LAST_STATUS}: ${LAST_BODY}"
}

log "1/4 login (${BASE_URL}/api/v1/auth/login)"
call_api -X POST "${BASE_URL}/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${DEMO_EMAIL}\",\"password\":\"${DEMO_PASSWORD}\"}"
expect_status "login" 200
TOKEN=$(extract_field "$LAST_BODY" "token")
[ -n "$TOKEN" ] || fail "no token in login response: ${LAST_BODY}"
log "  token acquired"

log "2/4 calculate (${BASE_URL}/api/v1/betslip/calculate)"
call_api -X POST "${BASE_URL}/api/v1/betslip/calculate" \
  -H 'Content-Type: application/json' \
  -d "{\"selectionIds\":[\"${SELECTION_ID}\"],\"stake\":10}"
expect_status "calculate" 200
case "$LAST_BODY" in
  *singles*) log "  calculate returned singles" ;;
  *) fail "calculate response missing 'singles': ${LAST_BODY}" ;;
esac

log "3/4 place (${BASE_URL}/api/v1/betslip/place)"
call_api -X POST "${BASE_URL}/api/v1/betslip/place" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Idempotency-Key: smoke-$(date +%s)" \
  -d "{\"selectionIds\":[\"${SELECTION_ID}\"],\"stake\":10}"
# A fresh Idempotency-Key against the seeded demo balance must always be
# accepted (201) — a 409 rejection here is a real failure, not a status
# this script may silently treat as "the request went through".
expect_status "place" 201
BET_STATUS=$(extract_field "$LAST_BODY" "status")
[ "$BET_STATUS" = "accepted" ] || fail "expected an accepted placement, got status=${BET_STATUS}: ${LAST_BODY}"
log "  bet status: ${BET_STATUS}"

log "4/4 balance (${BASE_URL}/api/v1/balance)"
call_api -X GET "${BASE_URL}/api/v1/balance" \
  -H "Authorization: Bearer ${TOKEN}"
expect_status "balance" 200
BALANCE=$(extract_field "$LAST_BODY" "balance")
[ -n "$BALANCE" ] || fail "no balance in balance response: ${LAST_BODY}"
log "  balance: ${BALANCE}"

log "OK — login, calculate, place, and balance all succeeded"
