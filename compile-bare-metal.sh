#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
MINIGOLF="${MINIGOLF:-$SCRIPT_DIR/minigolf}"
ASM6809="${ASM6809:-$SCRIPT_DIR/asm6809}"
BARE_LIB="$SCRIPT_DIR/bare-metal-lib"
BIGGOLFLIB="$SCRIPT_DIR/biggolflib"

# Ensure minigolf and asm6809 binaries exist
if [ ! -x "$MINIGOLF" ]; then
    echo "==> Building minigolf..." >&2
    (cd "$SCRIPT_DIR" && go build -o minigolf main.go)
fi
if [ ! -x "$ASM6809" ]; then
    echo "==> Building asm6809..." >&2
    (cd "$SCRIPT_DIR" && go build -o asm6809 ./cmd/asm6809)
fi

SRC="$1"
OUTPUT="$2"

if [ -z "$SRC" ]; then
    echo "Usage: $0 <input.golf> [output.decb]" >&2
    exit 1
fi

if [ -z "$OUTPUT" ]; then
    OUTPUT="${SRC%.*}.decb"
fi

OUT_DIR="$(dirname "$OUTPUT")"
mkdir -p "$OUT_DIR"
BASE="$(basename "$OUTPUT" .decb)"
TMP_ASM="$OUT_DIR/$BASE.asm"
TMP_LIST="$OUT_DIR/$BASE.list"

echo "==> Compiling $SRC -> $TMP_ASM (bare-metal gep9+)..." >&2
"$MINIGOLF" -m=6809+ -I="$BARE_LIB" -I="$BIGGOLFLIB" -o="$TMP_ASM" "$SRC"

echo "==> Assembling $TMP_ASM -> $OUTPUT..." >&2
"$ASM6809" --decb -l "$TMP_LIST" -o "$OUTPUT" "$TMP_ASM"
echo "==> Built $OUTPUT successfully." >&2
