#!/usr/bin/env bash
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
pids=()
trap 'kill "${pids[@]}" 2>/dev/null || true; rm -rf "$work"' EXIT

export PARVUS_SECRET=host-secret
export PARVUS_URL=http://127.0.0.1:9000
export LISTEN=127.0.0.1:8080
export APP_URL=http://localhost:8080

wait_for() {
  until curl --silent --fail --output /dev/null "$1"; do :; done
}

start_parvus() {
  rm -f "$work"/parvus.db*
  PARVUS_PEPPER=pepper \
  PARVUS_RP_ID=localhost \
  PARVUS_RP_NAME=Example \
  PARVUS_ORIGINS="$APP_URL" \
  PARVUS_DB="$work/parvus.db" \
  PARVUS_LISTEN=127.0.0.1:9000 \
  PARVUS_MAX_CREDENTIALS=20 \
    parvus >"$work/parvus.log" 2>&1 &
  pids+=($!)
  wait_for "$PARVUS_URL/healthz"
}

check_relay() {
  local name=$1 directory=$2
  shift 2
  start_parvus
  (cd "$root/examples/$directory" && exec "$@") >"$work/$name.log" 2>&1 &
  pids+=($!)
  wait_for "$APP_URL/"

  echo "# $name relay"
  node "$root/e2e/browser.mjs"

  kill "${pids[@]}"
  wait "${pids[@]}" 2>/dev/null || true
  pids=()
}

check_relay node relay-node node relay.mjs
check_relay go relay-go relay-go
