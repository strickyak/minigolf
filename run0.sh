#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
GEP0="$HATVAN_DIR/build/gep0"
if ! test -x "$GEP0"; then
    GEP0="$HATVAN_DIR/gep0"
fi
if ! test -x "$GEP0"; then
    GEP0="$SCRIPT_DIR/_tmp/gep0"
fi
MINIGOLF="$SCRIPT_DIR/minigolf"
ASM8086="$SCRIPT_DIR/asm8086"

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=8086 -o _tmp/0.asm "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.golf )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=8086 -o _tmp/0.asm "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.s | *.asm )
        cp -fv "$P" _tmp/0.asm >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .c, .golf, .s, or .asm: '$P'" >&2
        exit 13
        ;;
esac

test -x "$ASM8086" || ( cd "$SCRIPT_DIR" && go build -o asm8086 ./cmd/asm8086 )
"$ASM8086" -o _tmp/0.decb -l _tmp/0.list "$SCRIPT_DIR/i8086/cstart.asm" _tmp/0.asm >&2

if ! test -x "$GEP0"; then
    ( cd "$HATVAN_DIR" && go build -o "$SCRIPT_DIR/_tmp/gep0" ./cmd/gep0 )
    GEP0="$SCRIPT_DIR/_tmp/gep0"
fi

if test -z "$TRACE"
then
    "$GEP0" _tmp/0.decb
else
    "$GEP0" --trace _tmp/0.decb
fi
