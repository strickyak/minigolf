#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
BUILD_DIR="$DIR/build"
DECB="$BUILD_DIR/pyeval.decb"

# 1. Build pyeval.decb via Makefile
make -C "$DIR"

# 2. Locate gep9 emulator
GEP9="${GEP9:-}"
if [ -z "$GEP9" ]; then
    for cand in "$DIR/../../hatvan-os/build/gep9" "$DIR/../hatvan-os/build/gep9" "gep9"; do
        if [ -x "$cand" ]; then
            GEP9="$cand"
            break
        elif which "$cand" >/dev/null 2>&1; then
            GEP9="$(which "$cand")"
            break
        fi
    done
fi

if [ -z "$GEP9" ] || [ ! -x "$GEP9" ]; then
    echo "Error: gep9 emulator not found" >&2
    exit 1
fi

echo "=== Executing test suite in gep9 emulator ==="
INPUT_FILE="$DIR/test_input.txt"
WANT_FILE="$DIR/test_output.want"
ACTUAL_FILE="$BUILD_DIR/test_output.actual"

# Read commands from test_input.txt and feed to gep9
INPUT_STR=$(cat "$INPUT_FILE" | sed ':a;N;$!ba;s/\n/\\n/g')
"$GEP9" --embiggen --hypercalls -input="$INPUT_STR\n" "$DECB" > "$ACTUAL_FILE"

echo "=== Verifying output ==="
if diff -u "$WANT_FILE" "$ACTUAL_FILE"; then
    echo "SUCCESS: All Python evaluator tests passed on big6809 / gep9!"
else
    echo "FAILURE: Output mismatch!"
    exit 1
fi
