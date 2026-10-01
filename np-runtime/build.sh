#!/bin/bash
set -euo pipefail
NITROS9DIR="${NITROS9DIR:-/home/strick/modoc/coco-shelf/nitros9}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$SCRIPT_DIR"
lwasm.orig --no-warn=ifp1 --6809 --format=os9 \
    --pragma=pcaspcr,nosymbolcase,condundefzero,undefextern,dollarnotlocal,noforwardrefmax \
    --includedir=. --includedir="$NITROS9DIR/defs" \
    -o npcode npcode.asm

echo "Built npcode successfully ($(wc -c < npcode) bytes)."
