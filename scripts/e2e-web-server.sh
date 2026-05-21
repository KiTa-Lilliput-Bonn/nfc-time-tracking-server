#!/usr/bin/env bash
# Starts the Go server for Playwright E2E (test mode, temp DB).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/nfc-time-tracker-server"
if [[ ! -x "$BIN" ]]; then
  echo "E2E: Binary fehlt ($BIN) — zuerst: make build-with-web" >&2
  exit 1
fi

export NFC_TEST_MODE=1
export NFC_TEST_ADMIN_PASSWORD="${NFC_TEST_ADMIN_PASSWORD:-e2e-admin-password-min8}"
export NFC_AUTH_JWT_SECRET="${NFC_AUTH_JWT_SECRET:-e2e-jwt-secret-min-32-chars-long!!}"
export NFC_SERVER_HOST=127.0.0.1
export NFC_SERVER_PORT="${NFC_SERVER_PORT:-8091}"
export TZ="${TZ:-UTC}"

if [[ -z "${NFC_DATABASE_PATH:-}" ]]; then
  NFC_DATABASE_PATH="$(mktemp "${TMPDIR:-/tmp}/nfc-e2e-XXXXXX.db")"
  export NFC_DATABASE_PATH
fi
mkdir -p "$(dirname "$NFC_DATABASE_PATH")"

exec "$BIN"
