#!/bin/bash
# Beam shared build helpers — sourced by build-all.sh, publish.sh, packaging/*.
# Usage: APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"; source "$APP_DIR/packaging/common/lib.sh"
# Requires bash. Safe to source multiple times.

if [ -n "${BEAM_LIB_LOADED:-}" ]; then
  return 0 2>/dev/null || exit 0
fi
BEAM_LIB_LOADED=1

# load_ver <file>: print trimmed VERSION or fail with Arabic message.
beam_load_ver() {
  local f="${1:-VERSION}"
  local ver=""
  if [ -f "$f" ]; then
    ver="$(tr -d ' \t\r\n' < "$f" 2>/dev/null)"
  fi
  if [ -z "$ver" ]; then
    echo "❌ ملف VERSION مفقود أو فارغ ($f)." >&2
    return 1
  fi
  printf '%s' "$ver"
}

# require_cmd <cmd> [hint]: fail with Arabic message when missing.
beam_require_cmd() {
  local cmd="$1"
  local hint="${2:-}"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "❌ $cmd غير موجود. $hint" >&2
    return 1
  fi
}

# go_build <ver> <out> [GOOS] [GOARCH]: single canonical static build with version stamp.
# Caller must set GOOS/GOARCH via env or args. Uses GOPROXY=off, -trimpath, -buildvcs=false.
beam_go_build() {
  local ver="$1"
  local out="$2"
  local goos="${3:-${GOOS:-linux}}"
  local goarch="${4:-${GOARCH:-amd64}}"
  if [ -z "$ver" ] || [ -z "$out" ]; then
    echo "❌ beam_go_build: ver/out مطلوبان" >&2
    return 1
  fi
  beam_require_cmd go "ثبّته مرة واحدة من https://go.dev/dl" || return 1
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOPROXY=off \
    go -C goserver build -trimpath -buildvcs=false \
    -ldflags="-s -w -X fileshare.AppVersion=$ver" \
    -o "$out" ./cmd/beam
}

# beam_health_wait <port> [tries]: wait for /health (0 = ok).
beam_health_wait() {
  local port="${1:-2004}"
  local tries="${2:-30}"
  local i
  for i in $(seq 1 "$tries"); do
    if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$port/health" 2>/dev/null; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}
