#!/usr/bin/env bash
# Print random MDCP_JWT_SECRET and MDCP_ENCRYPTION_KEY values for local .env setup.
# Usage: ./scripts/generate-secrets.sh >> .env

set -euo pipefail

if ! command -v openssl >/dev/null 2>&1; then
  echo "error: openssl is required" >&2
  exit 1
fi

jwt_secret="$(openssl rand -base64 48 | tr -d '\n')"
encryption_key="$(openssl rand -base64 32 | tr -d '\n')"
admin_password="$(openssl rand -base64 16 | tr -d '\n/+=' | head -c 20)"

echo "MDCP_JWT_SECRET=${jwt_secret}"
echo "MDCP_ENCRYPTION_KEY=${encryption_key}"
echo "MDCP_ADMIN_PASSWORD=${admin_password}"
