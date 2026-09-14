set -ex

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HATVAN_DIR="$(cd "$SCRIPT_DIR/../hatvan-os" && pwd)"
HATVAN_VM="$HATVAN_DIR/hatvan-vm"

mkdir -p _tmp

P=$1
shift

case "$P" in 
    *.c )
        go run main.go -m M6809 -o _tmp/main.asm  "$@"  -I=tests -I=c-tests -I=demos -I=golflib "$P" >&2
        ;;
    *.golf )
        go run main.go -m M6809 -o _tmp/main.asm  "$@"  -I=tests -I=c-tests -I=demos -I=golflib "$P" >&2
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

cat >cstart.asm <<'HERE'
	pragma cescapes

    org $8000

cstart:
    lds  #$8000

cstart_continue_to_main:
    clra
    clrb
    tfr d,x
    tfr d,y
    tfr d,u
    lbsr _main

__exit0:
    clra              ; set exit status 0
    clrb
    tfr d,x

__exit:
    fcb  $12,$21,107  ; 1. Hyper Exit (with status in X)
    stb  $FF05        ; 2. Hatvan Exit port ($FF05)
stuck:
    bra stuck         ; 3. Infinite Loop

getchar:
_getchar:
    ; get byte in B or 0
    fcb  $12,$21,133  ; Hyper GetChar
    clra
    rts

putchar:
_putchar:
    clra
    fcb  $12,$21,132  ; Hyper PutChar
    rts

**  putchar:
**  _putchar:
**      ldb  2,s
**      clra
**      fcb  $12,$21,104  ; Hyper ShowChar
**      rts

_printf:
    leax 2,s
    fcb  $12,$21,111  ; Hyper Printf
    rts

f_prelude__mul_byte:
    lda 2,s   ; first byte arg a
    ldb 3,s   ; second byte arg b
    mul
    tfr d,x   ; leave result in D and X
    rts

percent_c:
    fcb '%,'c,0

    daa
    daa
    daa
    daa
    daa
    daa
    daa
    daa

    daa
    daa
    daa
    daa
    daa
    daa
    daa
    daa
HERE

cat cstart.asm main.asm > moto.asm
echo "    end cstart" >> moto.asm

time - lwasm --decb --list=moto.list -o moto.decb moto.asm

if test -s moto.decb; then
    DECB_SIZE=$(wc -c < moto.decb)
    PAYLOAD_SIZE=$((DECB_SIZE - 10))
    echo "[m6809 codesize: $PAYLOAD_SIZE]" >&2
fi

#############

test -s "$HATVAN_VM" || ( cd "$HATVAN_DIR" && go build -o hatvan-vm ./cmd/hatvan-vm )

if test -z "$TRACE"
then
    "$HATVAN_VM" --hypercalls --print-cycles moto.decb
else
    "$HATVAN_VM" --hypercalls --trace --print-cycles moto.decb moto.list
fi

