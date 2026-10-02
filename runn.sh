#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
HATVAN_VM="$HATVAN_DIR/hatvan-vm"
if test -x "$HATVAN_DIR/build/gep9"; then
    HATVAN_VM="$HATVAN_DIR/build/gep9"
elif test -f "$HATVAN_DIR/gep9" -a -x "$HATVAN_DIR/gep9"; then
    HATVAN_VM="$HATVAN_DIR/gep9"
fi
MINIGOLF="$SCRIPT_DIR/minigolf"
NPBUILD="$SCRIPT_DIR/np-runtime/npbuild.py"

mkdir -p _tmp

P=$1
if test -z "$P"; then
    echo "Usage: $0 <file.golf|file.npasm|file.npc> [options]" >&2
    exit 1
fi
shift

# Build minigolf compiler binary if missing or stale
if [ ! -x "$MINIGOLF" ] || [ "$SCRIPT_DIR/np/codegen.go" -nt "$MINIGOLF" ] || [ "$SCRIPT_DIR/main.go" -nt "$MINIGOLF" ]; then
    ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
fi

# Determine execution mode: verify vs run
RUN_MODE="--run"
if test -n "$VERIFY"; then
    RUN_MODE="--verify"
fi

VERBOSE_FLAG=()
if test -n "$TRACE" || test -n "$VERBOSE"; then
    VERBOSE_FLAG=(-v)
fi

case "$P" in 
    *.golf | *.npasm | *.npc )
        python3 "$NPBUILD" "$P" \
            "$RUN_MODE" \
            "${VERBOSE_FLAG[@]}" \
            -I "$SCRIPT_DIR/np-lib" \
            -I "$SCRIPT_DIR/tests" \
            -I "$SCRIPT_DIR/golflib" \
            --gep9 "$HATVAN_VM" \
            "$@"
        ;;
    * )
        echo "BAD EXTENSION: Expected .golf, .npasm, or .npc: '$P'" >&2
        exit 13
        ;;
esac
