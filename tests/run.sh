#!/usr/bin/env bash
set -euo pipefail
if [[ "${TARN_TEST_CPU_BOUND:-0}" != "1" ]]; then
  cpu=$(python3 -c 'import os; print(min(os.sched_getaffinity(0)))')
  export TARN_TEST_CPU_BOUND=1 TARN_TEST_CPU="$cpu"
  exec nice -n 15 taskset -c "$cpu" bash "$0" "$@"
fi
export GOMAXPROCS=1
export GOFLAGS="${GOFLAGS:-} -p=1"
root=$(cd "$(dirname "$0")/.." && pwd)
tarn=${1:-tarn}
work=$(mktemp -d)
container=""
server_pid=""
cleanup() {
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; fi
  if [[ -n "$container" ]]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
if [[ "$tarn" == "--docker" ]]; then
  container="tarn-grpc-tests-${work##*/}"
  docker run --cpu-shares=128 --cpus=1 --cpuset-cpus="$TARN_TEST_CPU" --name "$container" -v "$root:/source:ro" \
    ghcr.io/tarn-lng/tarn:0.2.0@sha256:c078e123fad6158bfd27e43b830a9571e899c3073a025b057e04f5018c645e6a \
    sh -c 'mkdir -p /app && cp -a /source/. /app/ && cd /app && tarn fetch && tarn verify && tarn check lib.tarn && tarn test && tarn build integration.tarn -o /tmp/integration'
  docker cp "$container:/tmp/integration" "$work/integration" >/dev/null
else
  (cd "$root" && "$tarn" fetch && "$tarn" verify && "$tarn" check lib.tarn && "$tarn" test && "$tarn" build integration.tarn -o "$work/integration")
fi
(cd "$root/tests/grpcserver" && GOPROXY=off go build -o "$work/server" .)
"$work/server" > "$work/address" &
server_pid=$!
for _ in $(seq 50); do [ -s "$work/address" ] && break; sleep 0.1; done
if [[ ! -s "$work/address" ]]; then echo "test server did not start" >&2; exit 1; fi
timeout 60 "$work/integration" "$(head -1 "$work/address")"
