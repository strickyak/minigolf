#!/bin/sh
set -ex

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
HATVAN_VM="$HATVAN_DIR/hatvan-vm"
if test -x "$HATVAN_DIR/build/gep9"; then
    HATVAN_VM="$HATVAN_DIR/build/gep9"
elif test -x "$HATVAN_DIR/build/gap9"; then
    HATVAN_VM="$HATVAN_DIR/build/gap9"
elif test -f "$HATVAN_DIR/gep9" -a -x "$HATVAN_DIR/gep9"; then
    HATVAN_VM="$HATVAN_DIR/gep9"
elif test -f "$HATVAN_DIR/gap9" -a -x "$HATVAN_DIR/gap9"; then
    HATVAN_VM="$HATVAN_DIR/gap9"
fi

if ! test -x "$SCRIPT_DIR/asm6809"; then
    ( cd "$SCRIPT_DIR" && go build -o asm6809 ./cmd/asm6809 )
fi

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        go run "$SCRIPT_DIR/main.go" -m 6809+ -o _tmp/main.asm  "$@"  -I=biggolflib -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.golf )
        go run "$SCRIPT_DIR/main.go" -m 6809+ -o _tmp/main.asm  "$@"  -I=biggolflib -I=tests -I=c-tests -I=c-demos -I=c-demos/floating -I=c-demos/pythonsub -I=demos -I=demos/floating -I=golflib "$P" >&2
        ;;
    *.s | *.asm )
        cp -fv "$P" _tmp/main.asm >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .golf or .s or .asm: '$P'" >&2
        exit 13
        ;;
esac

cd _tmp

cp main.asm moto.asm

if test -x "$SCRIPT_DIR/asm6809"; then
    time - "$SCRIPT_DIR/asm6809" --decb --list=moto.list -o moto.decb moto.asm
elif which asm6809 >/dev/null 2>&1; then
    time - asm6809 --decb --list=moto.list -o moto.decb moto.asm
else
    time - lwasm --decb --list=moto.list -o moto.decb moto.asm
fi

if test -s moto.decb; then
    DECB_SIZE=$(wc -c < moto.decb)
    PAYLOAD_SIZE=$((DECB_SIZE - 10))
    echo "[m6809 codesize: $PAYLOAD_SIZE]" >&2
fi

#############

if ! test -s "$HATVAN_VM"; then
    ( cd "$HATVAN_DIR" && (go build -o build/gep9 ./cmd/gep9 || go build -o hatvan-vm ./cmd/hatvan-vm) )
    if test -s "$HATVAN_DIR/build/gep9"; then
        HATVAN_VM="$HATVAN_DIR/build/gep9"
    fi
fi

if test -z "$TRACE"
then
    "$HATVAN_VM" --embiggen --hypercalls --print-cycles moto.decb
else
    "$HATVAN_VM" --embiggen --hypercalls --trace --print-cycles moto.decb moto.list
fi
