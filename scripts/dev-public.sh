#!/usr/bin/env bash
# Public HTTPS URL for the desk. Backend (:8080) and Next (:43127) must already
# be up. The browser talks to this origin only; /gw is proxied to the gateway.
# Anyone with the URL can drive the paper book — do not paste it in a ticket.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${PORT:-43127}"
DESK="http://127.0.0.1:${PORT}"

if ! curl -sf -o /dev/null "${DESK}/"; then
  echo "desk not listening on :${PORT}. Start: bash scripts/dev-backend.sh &  make web" >&2
  exit 1
fi
if ! curl -sf -o /dev/null "${DESK}/gw/health"; then
  echo "GET ${DESK}/gw/health failed. Is the gateway up on :8080?" >&2
  exit 1
fi

if ! command -v cloudflared >/dev/null 2>&1; then
  echo "cloudflared not on PATH. Install: brew install cloudflare/cloudflare/cloudflared" >&2
  exit 1
fi

echo "tunneling ${DESK} — leave this running; Ctrl-C closes the public URL"
# Rewrite Host so Next.js accepts the request; the public hostname is in the URL.
exec cloudflared tunnel --no-autoupdate --url "${DESK}" --http-host-header "127.0.0.1:${PORT}"
