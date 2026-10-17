#!/usr/bin/env bash
# Idempotent dependency install for Cloud Agents. Toolchains live in the image.
set -euo pipefail

export PATH="/usr/local/go/bin:/usr/local/bin:${PATH}"

cd "$(dirname "${BASH_SOURCE[0]}")/.."

go mod download
npm ci --prefix frontend
