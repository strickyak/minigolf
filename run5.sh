#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
GEP5="$HATVAN_DIR/build/gep5"
if ! test -x "$GEP5"; then
    GEP5="$HATVAN_DIR/gep5"
fi
if ! test -x "$GEP5"; then
    GEP5="$SCRIPT_DIR/_tmp/gep5"
fi
MINIGOLF="$SCRIPT_DIR/minigolf"
ASM6502="$SCRIPT_DIR/asm6502"

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=6502 -o _tmp/5.asm "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.golf )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=6502 -o _tmp/5.asm "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.s | *.asm )
        cp -fv "$P" _tmp/5.asm >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .c, .golf, .s, or .asm: '$P'" >&2
        exit 13
        ;;
esac

test -x "$ASM6502" || ( cd "$SCRIPT_DIR" && go build -o asm6502 ./cmd/asm6502 )
"$ASM6502" -o _tmp/5.decb -l _tmp/5.list _tmp/5.asm >&2

if ! test -x "$GEP5"; then
    ( cd "$HATVAN_DIR" && go build -o "$SCRIPT_DIR/_tmp/gep5" ./cmd/gep5 )
    GEP5="$SCRIPT_DIR/_tmp/gep5"
fi

if test -z "$TRACE"
then
    "$GEP5" _tmp/5.decb
else
    "$GEP5" --trace _tmp/5.decb
fi
