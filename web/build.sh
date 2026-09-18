#!/usr/bin/env bash
# Builds the browser bundle into web/dist, which is what gets deployed.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
out="$root/web/dist"
mkdir -p "$out"
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o "$out/main.wasm" "$root/cmd/wasm"
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$out/wasm_exec.js"
cp "$root/web/index.html" "$out/index.html"
ls -la "$out"
