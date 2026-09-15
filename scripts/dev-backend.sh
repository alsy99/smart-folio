#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p "$ROOT/pids" "$ROOT/data"
export AUTOSTART_CAMPAIGN="${AUTOSTART_CAMPAIGN:-true}"
export INVESTIGATION_LLM="${INVESTIGATION_LLM:-true}"
export SCALP_MODE="${SCALP_MODE:-false}"
export PATH="$PATH:$(go env GOPATH)/bin"

# Paper fills follow NSE 09:15–15:30 IST unless MARKET_CLOCK_OVERRIDE=open.

# Pull keys from a login zsh (e.g. ~/.zshrc) when this script's env is empty.
for k in NEWSAPI_KEY GOOGLE_API_KEY GEMINI_API_KEY OPENAI_API_KEY GROQ_API_KEY NVIDIA_API_KEY ZAI_API_KEY OPENROUTER_API_KEY MISTRAL_API_KEY INDSTOCKS_ACCESS_TOKEN INDSTOCKS_API_KEY INDMONEY_API_TOKEN INDMONEY_CLIENT_ID; do
  if [[ -z "${!k:-}" ]]; then
    export "$k=$(zsh -lic "printf %s \"\${$k-}\"")"
  fi
done
if [[ -z "${INDSTOCKS_ACCESS_TOKEN:-}" && -n "${INDMONEY_API_TOKEN:-}" ]]; then
  export INDSTOCKS_ACCESS_TOKEN="$INDMONEY_API_TOKEN"
fi
if [[ -z "${INDSTOCKS_API_KEY:-}" && -n "${INDMONEY_CLIENT_ID:-}" ]]; then
  export INDSTOCKS_API_KEY="$INDMONEY_CLIENT_ID"
fi

echo "keys: newsapi=${NEWSAPI_KEY:+yes} google=${GOOGLE_API_KEY:+yes} openai=${OPENAI_API_KEY:+yes} groq=${GROQ_API_KEY:+yes} mistral=${MISTRAL_API_KEY:+yes} openrouter=${OPENROUTER_API_KEY:+yes} nvidia=${NVIDIA_API_KEY:+yes} zai=${ZAI_API_KEY:+yes} indstocks=${INDSTOCKS_ACCESS_TOKEN:+yes}"
echo "clock: nse-hours override=${MARKET_CLOCK_OVERRIDE:-off} investigation_llm=${INVESTIGATION_LLM:-true} scalp=${SCALP_MODE:-false}"

free_port() {
  local port="$1"
  local ids
  ids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -n "$ids" ]]; then
    echo "freeing :$port"
    # shellcheck disable=SC2086
    kill $ids 2>/dev/null || true
    sleep 0.3
    ids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$ids" ]]; then
      # shellcheck disable=SC2086
      kill -9 $ids 2>/dev/null || true
    fi
  fi
}

wait_port() {
  local port="$1"
  local name="$2"
  local i=0
  while (( i < 90 )); do
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.3
    i=$((i + 1))
  done
  echo "$name failed to listen on :$port" >&2
  tail -n 30 "$ROOT/data/$name.log" >&2 || true
  return 1
}

stop() {
  if [[ -f pids/backend.pids ]]; then
    while read -r pid; do
      kill "$pid" 2>/dev/null || true
      pkill -P "$pid" 2>/dev/null || true
    done < pids/backend.pids
    rm -f pids/backend.pids
  fi
  for p in 9081 9082 9083 9084 9085 8080; do
    free_port "$p"
  done
}

for p in 9081 9082 9083 9084 9085 8080; do
  free_port "$p"
done
trap stop EXIT

start() {
  local name="$1"
  local port="$2"
  echo "starting $name"
  go run "$ROOT/cmd/$name" >"$ROOT/data/$name.log" 2>&1 &
  echo $! >> pids/backend.pids
  wait_port "$port" "$name"
}

: > pids/backend.pids
start marketdata 9081
start learning 9083
start sentiment 9084
start advisor 9085
start trading 9082
start gateway 8080

echo "backend up — gateway http://127.0.0.1:8080"
wait
