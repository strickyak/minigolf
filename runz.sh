#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
GEPZ="$HATVAN_DIR/build/gepz"
if ! test -x "$GEPZ"; then
    GEPZ="$HATVAN_DIR/gepz"
fi
MINIGOLF="$SCRIPT_DIR/minigolf"
ASMZ80="$SCRIPT_DIR/asmz80"

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=z80 -o _tmp/main.s "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.golf )
        test -x "$MINIGOLF" || ( cd "$SCRIPT_DIR" && go build -o minigolf main.go )
        "$MINIGOLF" -m=z80 -o _tmp/main.s "$@" -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.s | *.asm )
        cp -fv "$P" _tmp/main.s >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .c, .golf, .s, or .asm: '$P'" >&2
        exit 13
        ;;
esac

test -x "$ASMZ80" || ( cd "$SCRIPT_DIR" && go build -o asmz80 ./cmd/asmz80 )
cat "$SCRIPT_DIR/z80/cstart.asm" _tmp/main.s > _tmp/full.asm
"$ASMZ80" -o _tmp/z.decb -l _tmp/z.list _tmp/full.asm >&2

if ! test -s "$GEPZ"; then
    ( cd "$HATVAN_DIR" && go build -o build/gepz ./cmd/gepz )
    GEPZ="$HATVAN_DIR/build/gepz"
fi

if test -z "$TRACE"
then
    "$GEPZ" _tmp/z.decb
else
    "$GEPZ" --trace _tmp/z.decb
fi
