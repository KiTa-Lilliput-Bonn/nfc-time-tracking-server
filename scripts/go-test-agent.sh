#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -eq 0 ]; then
  set -- ./...
fi

: "${HOME:?HOME must be set}"

export GOMODCACHE="${GOMODCACHE:-$HOME/go/pkg/mod}"
export GOCACHE="${GOCACHE:-$HOME/go-build-cache-cursor}"

mkdir -p "$GOCACHE"

exec go test "$@"
