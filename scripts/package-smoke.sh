#!/bin/sh
set -eu

binary=${1:-./bin/n0ding-bench}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
"$binary" init --db "$work/bench.db" >/dev/null
"$binary" serve --addr 127.0.0.1:18080 --db "$work/bench.db" >"$work/server.log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; rm -rf "$work"' EXIT
i=0
until "$binary" doctor --url http://127.0.0.1:18080 >/dev/null 2>&1; do
  i=$((i+1)); [ "$i" -lt 50 ] || { cat "$work/server.log"; exit 1; }
  sleep 0.1
done
kill "$pid"
wait "$pid" 2>/dev/null || true
