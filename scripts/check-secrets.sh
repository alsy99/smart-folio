#!/usr/bin/env bash
# Fail if NEXT_PUBLIC_* grows an API key, token, or secret into the browser bundle.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if grep -R -n -E 'NEXT_PUBLIC_.+(KEY|TOKEN|SECRET|PASSWORD|MPIN)' \
  --include='*.ts' --include='*.tsx' --include='*.js' --include='*.mjs' --include='*.env*' \
  "$ROOT/apps/web" "$ROOT/.env.example" 2>/dev/null; then
  echo "NEXT_PUBLIC_* must not carry API keys; secrets stay in server env." >&2
  exit 1
fi
echo "secrets: NEXT_PUBLIC_* has no keys"
