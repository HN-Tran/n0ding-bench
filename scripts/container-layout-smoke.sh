#!/bin/sh
set -eu

image=${1:-n0ding-bench:ci}
token=ci-smoke-token
work=$(mktemp -d)
volume=bench-smoke-data-$$
cleanup() {
  docker rm -f bench-smoke-raw bench-smoke-volume >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

wait_healthy() {
  name=$1
  port=$2
  i=0
  until curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; do
    i=$((i+1))
    if [ "$i" -ge 50 ]; then docker logs "$name"; exit 1; fi
    sleep 0.2
  done
  [ "$(docker inspect -f '{{.State.Running}}' "$name")" = true ]
  curl -fsS -H "Authorization: Bearer $token" -X POST "http://127.0.0.1:${port}/api/v1/fixtures" | grep -q '"id":"bench-fixture"'
  curl -fsS -H "Authorization: Bearer $token" "http://127.0.0.1:${port}/api/v1/runs/bench-fixture/projection" | grep -q '"status":"completed"'
  docker cp "$name:/data/bench.db" "$work/${name}.db"
  test -s "$work/${name}.db"
}

# Raw image-layer startup proves /data itself is owned by the nonroot runtime.
docker run -d --name bench-smoke-raw -p 18080:8080 -e N0DING_BENCH_AUTH_TOKEN="$token" "$image" >/dev/null
wait_healthy bench-smoke-raw 18080
docker rm -f bench-smoke-raw >/dev/null

# Named-volume startup mirrors compose hardening and proves volume copy-up keeps
# the writable /data ownership under a read-only root filesystem.
docker volume create "$volume" >/dev/null
docker run -d --name bench-smoke-volume -p 18081:8080 --read-only --tmpfs /tmp:size=64m,mode=1777 \
  --security-opt no-new-privileges:true -v "$volume:/data" \
  -e N0DING_BENCH_AUTH_TOKEN="$token" "$image" >/dev/null
wait_healthy bench-smoke-volume 18081
