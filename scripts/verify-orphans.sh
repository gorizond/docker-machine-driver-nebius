#!/usr/bin/env bash
# Lists instances owned by docker-machine-driver-nebius in the Nebius project.
# Exit codes: 0 = no orphans, 1 = orphans remain, 2 = configuration/API error.
#
# Required env:
#   NEBIUS_SA_ID, NEBIUS_PUBLIC_KEY_ID, NEBIUS_PRIVATE_KEY_PATH, NEBIUS_PROJECT_ID
# Optional env:
#   NEBIUS_API_ENDPOINT, NEBIUS_CLUSTER_ID
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
exec go run ./cmd/orphans-check
