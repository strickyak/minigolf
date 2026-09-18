#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
HATVAN_VMK="$HATVAN_DIR/hatvan-vmk"
MINIGOLF="$SCRIPT_DIR/minigolf"
ASM68K="$SCRIPT_DIR/asm68k"

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=k -o _tmp/main.s "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.golf )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=k -o _tmp/main.s "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.s | *.asm )
        cp -fv "$P" _tmp/main.s >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .c, .golf, .s, or .asm: '$P'" >&2
        exit 13
        ;;
esac

test -x "$ASM68K" || ( cd "$SCRIPT_DIR" && go build -o asm68k ./cmd/asm68k )
"$ASM68K" -o _tmp/moto.srec "$SCRIPT_DIR/m68k/cstart.asm" _tmp/main.s >&2

test -s "$HATVAN_VMK" || ( cd "$HATVAN_DIR" && go build -o hatvan-vmk ./cmd/hatvan-vmk )

if test -z "$TRACE"
then
    "$HATVAN_VMK" --print-cycles _tmp/moto.srec
else
    "$HATVAN_VMK" --trace --print-cycles _tmp/moto.srec
fi
