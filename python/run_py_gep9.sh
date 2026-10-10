#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
BUILD_DIR="$DIR/build"
DECB="$BUILD_DIR/pyeval.decb"

# Ensure binary is built
if [ ! -f "$DECB" ]; then
    echo "Building pyeval in $BUILD_DIR..." >&2
    make -C "$DIR"
fi

# Locate gep9 emulator
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

exec "$GEP9" --embiggen --hypercalls "$DECB" "$@"
