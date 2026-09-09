#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p "$ROOT/pids" "$ROOT/data"
export MARKET_CLOCK_OVERRIDE="${MARKET_CLOCK_OVERRIDE:-open}"
export AUTOSTART_CAMPAIGN="${AUTOSTART_CAMPAIGN:-true}"
export PATH="$PATH:$(go env GOPATH)/bin"

stop() {
  if [[ -f pids/backend.pids ]]; then
    while read -r pid; do
      kill "$pid" 2>/dev/null || true
    done < pids/backend.pids
    rm -f pids/backend.pids
  fi
}
trap stop EXIT

start() {
  local name="$1"
  shift
  echo "starting $name"
  go run "$ROOT/services/$name" "$@" >"$ROOT/data/$name.log" 2>&1 &
  echo $! >> pids/backend.pids
}

: > pids/backend.pids
start marketdata
start learning
start sentiment
start advisor
sleep 1
start trading
sleep 1
start gateway

echo "backend up — gateway http://127.0.0.1:8080"
wait
