#!/usr/bin/env bash
# Run BenchmarkRulesEngineFullCoverage with the correct librure for the current platform.
# Usage:
#   ./bench.sh                          # default 10m
#   BENCHTIME=30s ./bench.sh            # override duration via env
#   ./bench.sh -bench='.../Parallel'    # extra go test flags forwarded verbatim
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LIB_ROOT="$REPO_ROOT/lib"

# ── platform / arch detection ────────────────────────────────────────────────
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Darwin)
    LIB_DIR="$LIB_ROOT/darwin"
    LDFLAGS="-L$LIB_DIR -lrure"
    ;;
  Linux)
    case "$ARCH" in
      x86_64)  LIB_DIR="$LIB_ROOT/linux/amd64" ;;
      aarch64) LIB_DIR="$LIB_ROOT/linux/arm64" ;;
      *)       echo "Unsupported Linux arch: $ARCH" >&2; exit 1 ;;
    esac
    LDFLAGS="-L$LIB_DIR -lrure -Wl,-rpath,$LIB_DIR"
    export LD_LIBRARY_PATH="${LIB_DIR}${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    ;;
  *)
    echo "Unsupported OS: $OS" >&2; exit 1
    ;;
esac

echo "Platform  : $OS/$ARCH"
echo "lib dir   : $LIB_DIR"

# ── benchmark parameters (override via env) ───────────────────────────────────
BENCH="${BENCH:-BenchmarkRulesEngine}"
BENCHTIME="${BENCHTIME:-10m}"

echo "Benchmark : $BENCH"
echo "Duration  : $BENCHTIME"
echo "────────────────────────────────────────────────────────"

export CGO_ENABLED=1
export CGO_LDFLAGS="$LDFLAGS"

cd "$SCRIPT_DIR/.."   # src/ — so `./rules_engine` package path resolves correctly

go test ./rules_engine \
  -run='^$' \
  -bench="$BENCH" \
  -benchtime="$BENCHTIME" \
  -count=1 \
  -benchmem \
  "$@"
