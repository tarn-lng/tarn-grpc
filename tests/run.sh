#!/usr/bin/env bash
# Integration against an independent peer: Go net/http grpc-go over plaintext HTTP/2.
# Usage: tests/run.sh [path/to/tarn]   (needs Go >= 1.24; builds offline)
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
tarn=${1:-tarn}
work=$(mktemp -d)
trap 'kill "${server_pid:-}" 2>/dev/null || true; rm -rf "$work"' EXIT
(cd "$root/tests/grpcserver" && GOFLAGS=-mod=mod GOPROXY=off go build -o "$work/grpcserver" .)
(cd "$root" && "$tarn" build integration.tarn -o "$work/integration")
"$work/grpcserver" > "$work/address" &
server_pid=$!
for _ in $(seq 50); do [ -s "$work/address" ] && break; sleep 0.1; done
timeout 60 "$work/integration" "$(head -1 "$work/address")"
