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

log "1/4 login (${BASE_URL}/api/v1/auth/login)"
login_body=$(curl -sS -X POST "${BASE_URL}/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${DEMO_EMAIL}\",\"password\":\"${DEMO_PASSWORD}\"}")
TOKEN=$(extract_field "$login_body" "token")
[ -n "$TOKEN" ] || fail "no token in login response: ${login_body}"
log "  token acquired"

log "2/4 calculate (${BASE_URL}/api/v1/betslip/calculate)"
calc_body=$(curl -sS -X POST "${BASE_URL}/api/v1/betslip/calculate" \
  -H 'Content-Type: application/json' \
  -d "{\"selectionIds\":[\"${SELECTION_ID}\"],\"stake\":10}")
case "$calc_body" in
  *singles*) log "  calculate returned singles" ;;
  *) fail "calculate response missing 'singles': ${calc_body}" ;;
esac

log "3/4 place (${BASE_URL}/api/v1/betslip/place)"
place_body=$(curl -sS -X POST "${BASE_URL}/api/v1/betslip/place" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Idempotency-Key: smoke-$(date +%s)" \
  -d "{\"selectionIds\":[\"${SELECTION_ID}\"],\"stake\":10}")
BET_STATUS=$(extract_field "$place_body" "status")
[ -n "$BET_STATUS" ] || fail "no status in place response: ${place_body}"
log "  bet status: ${BET_STATUS}"

log "4/4 balance (${BASE_URL}/api/v1/balance)"
balance_body=$(curl -sS -X GET "${BASE_URL}/api/v1/balance" \
  -H "Authorization: Bearer ${TOKEN}")
BALANCE=$(extract_field "$balance_body" "balance")
[ -n "$BALANCE" ] || fail "no balance in balance response: ${balance_body}"
log "  balance: ${BALANCE}"

log "OK — login, calculate, place, and balance all succeeded"
