#!/usr/bin/env bash
# Task 20: minimal automated smoke — fresh DB, health, SPA shell, admin login.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="$ROOT/bin/nfc-time-tracker-server"
test -x "$BIN" || { echo "Binary fehlt: $BIN — zuerst: make build oder make build-with-web"; exit 1; }

TMP="$(mktemp -d)"
cleanup() {
  if [ -n "${SRV_PID:-}" ]; then
    kill "$SRV_PID" 2>/dev/null || true
    wait "$SRV_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

export NFC_DATABASE_PATH="$TMP/smoke.db"
export NFC_SERVER_PORT="${NFC_SMOKE_PORT:-18080}"
export NFC_SERVER_HOST="127.0.0.1"
export NFC_AUTH_JWT_SECRET="smoke-e2e-jwt-secret-key-min-32chars!"

"$BIN" >"$TMP/server.log" 2>&1 &
SRV_PID=$!

ok=0
for _ in $(seq 1 80); do
  if curl -sf "http://${NFC_SERVER_HOST}:${NFC_SERVER_PORT}/api/v1/health" >/dev/null; then
    ok=1
    break
  fi
  sleep 0.05
done
if [ "$ok" != 1 ]; then
  echo "Health-Check Timeout. Log:"
  cat "$TMP/server.log"
  exit 1
fi

HTML="$(curl -sf "http://${NFC_SERVER_HOST}:${NFC_SERVER_PORT}/")"
echo "$HTML" | grep -qiE 'vite|id=.app|root' || {
  echo "SPA-Root scheint nicht geladen (kein typisches index.html-Markup)."
  echo "$HTML" | head -c 400
  exit 1
}

DEEP="$(curl -sf "http://${NFC_SERVER_HOST}:${NFC_SERVER_PORT}/reports")"
echo "$DEEP" | grep -qiE 'vite|id=.app|root' || {
  echo "SPA-Fallback für /reports fehlgeschlagen."
  exit 1
}

PW=""
if grep -q 'one-time password:' "$TMP/server.log" 2>/dev/null; then
  PW="$(grep 'one-time password:' "$TMP/server.log" | head -1 | sed 's/.*one-time password: //' | tr -d '\r')"
fi
if [ -z "$PW" ]; then
  PW="${NFC_SMOKE_ADMIN_PW:-}"
fi
if [ -z "$PW" ]; then
  echo "Kein Einmalpasswort in Log und NFC_SMOKE_ADMIN_PW nicht gesetzt — Login-Skip (Health+SPA ok)."
  echo "Smoke: health + SPA + deep link OK."
  exit 0
fi

BODY="$(curl -sf -X POST "http://${NFC_SERVER_HOST}:${NFC_SERVER_PORT}/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PW\"}")"
echo "$BODY" | grep -q '"token"' || {
  echo "Login fehlgeschlagen: $BODY"
  exit 1
}

echo "Smoke: health + SPA + /reports fallback + admin login OK."
