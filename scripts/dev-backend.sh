#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p "$ROOT/pids" "$ROOT/data"
export AUTOSTART_CAMPAIGN="${AUTOSTART_CAMPAIGN:-true}"
export INVESTIGATION_LLM="${INVESTIGATION_LLM:-true}"
export PATH="$PATH:$(go env GOPATH)/bin"

# Paper fills follow NSE 09:15–15:30 IST unless MARKET_CLOCK_OVERRIDE=open.

# Pull keys from a login zsh (e.g. ~/.zshrc) when this script's env is empty.
for k in NEWSAPI_KEY GOOGLE_API_KEY GEMINI_API_KEY OPENAI_API_KEY GROQ_API_KEY NVIDIA_API_KEY ZAI_API_KEY OPENROUTER_API_KEY MISTRAL_API_KEY; do
  if [[ -z "${!k:-}" ]]; then
    export "$k=$(zsh -lic "printf %s \"\${$k-}\"")"
  fi
done

echo "keys: newsapi=${NEWSAPI_KEY:+yes} google=${GOOGLE_API_KEY:+yes} openai=${OPENAI_API_KEY:+yes} groq=${GROQ_API_KEY:+yes} mistral=${MISTRAL_API_KEY:+yes} openrouter=${OPENROUTER_API_KEY:+yes} nvidia=${NVIDIA_API_KEY:+yes} zai=${ZAI_API_KEY:+yes}"
echo "clock: nse-hours override=${MARKET_CLOCK_OVERRIDE:-off} investigation_llm=${INVESTIGATION_LLM:-true}"

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
  go run "$ROOT/cmd/$name" "$@" >"$ROOT/data/$name.log" 2>&1 &
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
