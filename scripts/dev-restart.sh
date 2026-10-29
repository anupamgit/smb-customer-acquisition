#!/usr/bin/env bash
# Local dev: stop prior API on PORT, rebuild web, run API (loads repo-root .env).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi
PORT="${PORT:-8080}"

if command -v lsof >/dev/null 2>&1; then
  PIDS=$(lsof -ti "tcp:${PORT}" -sTCP:LISTEN 2>/dev/null || true)
  if [[ -n "${PIDS}" ]]; then
    echo "Stopping process(es) on port ${PORT}: ${PIDS}"
    kill ${PIDS} 2>/dev/null || true
    sleep 1
  fi
fi

echo "Building web..."
(cd src/web && npm run build)

echo "Starting API on :${PORT}..."
cd src/api
exec go run .
