#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
HATVAN_VMK="$HATVAN_DIR/hatvan-vmk"
if test -s "$HATVAN_DIR/build/gepk"; then
    HATVAN_VMK="$HATVAN_DIR/build/gepk"
elif test -s "$HATVAN_DIR/gepk"; then
    HATVAN_VMK="$HATVAN_DIR/gepk"
fi
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
"$ASM68K" -o _tmp/moto.srec -l _tmp/moto.list "$SCRIPT_DIR/m68k/cstart.asm" _tmp/main.s >&2

if ! test -s "$HATVAN_VMK"; then
    ( cd "$HATVAN_DIR" && (go build -o build/gepk ./cmd/gepk || go build -o hatvan-vmk ./cmd/gepk) )
    if test -s "$HATVAN_DIR/build/gepk"; then
        HATVAN_VMK="$HATVAN_DIR/build/gepk"
    fi
fi

if test -z "$TRACE"
then
    "$HATVAN_VMK" --print-cycles _tmp/moto.srec
else
    "$HATVAN_VMK" --trace --print-cycles _tmp/moto.srec
fi
