#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../backend"

go vet ./...

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "unformatted Go files:" >&2
  echo "$unformatted" >&2
  exit 1
fi
