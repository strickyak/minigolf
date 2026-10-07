#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
GEPC="$HATVAN_DIR/build/gepc"
if ! test -x "$GEPC"; then
    GEPC="$HATVAN_DIR/gepc"
fi
MINIGOLF="$SCRIPT_DIR/minigolf"
ASM1802="$SCRIPT_DIR/asm1802"

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=1802 -o _tmp/c.asm "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.golf )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=1802 -o _tmp/c.asm "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.s | *.asm )
        cp -fv "$P" _tmp/c.asm >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .c, .golf, .s, or .asm: '$P'" >&2
        exit 13
        ;;
esac

test -x "$ASM1802" || ( cd "$SCRIPT_DIR" && go build -o asm1802 ./cmd/asm1802 )
"$ASM1802" -o _tmp/c.decb -l _tmp/c.list _tmp/c.asm >&2

if ! test -s "$GEPC"; then
    ( cd "$HATVAN_DIR" && go build -o build/gepc ./cmd/gepc )
    GEPC="$HATVAN_DIR/build/gepc"
fi

if test -z "$TRACE"
then
    "$GEPC" _tmp/c.decb
else
    "$GEPC" --trace _tmp/c.decb
fi
