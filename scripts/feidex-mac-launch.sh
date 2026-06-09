#!/bin/sh
set -eu

LOG_PREFIX="/tmp/feidex"
REPO_DIR="/Volumes/Second HD/proj/feidex"

{
  date
  pwd
  env | sort
  echo "--- node ---"
  /opt/homebrew/bin/node --version
  echo "--- codex ---"
  /opt/homebrew/bin/codex --version
  echo "--- feidex ---"
} >"${LOG_PREFIX}.wrapper.log" 2>&1

exec "${REPO_DIR}/bin/feidex" serve --config "${REPO_DIR}/config.toml" >>"${LOG_PREFIX}.stdout.log" 2>>"${LOG_PREFIX}.stderr.log"
