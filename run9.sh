set -ex

P=$1
shift

case "$P" in 
    *.golf )
        go run main.go -m M6809 -o _tmp/main.asm  -I golflib "$@" "$P" >&2
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

    daa
    daa
    daa
    daa
    daa
    daa
    daa
    daa

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
    fcb 1             ; 2. Illegal Instruction
    nop
    nop
    nop
stuck:
    bra stuck         ; 3. Infinite Loop

putchar:
_putchar:
    ; first byte arg is already in B
    clra
    fcb  $12,$21,104  ; Hyper ShowChar
    rts

_printf:
    leax 2,s
    fcb  $12,$21,111  ; Hyper Printf
    rts

f_prelude__mul_byte:
    ; first byte arg is already in B
    lda 2,s   ; get second byte arg
    mul
    tfr d,x   ; leave result in X
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

time - lwasm --format=raw -o'moto.rom' moto.asm

#############

if test -z "$TRACE"
then
    test -s /home/strick/modoc/coco-shelf/gomar/gomar0n || \
    ( cd /home/strick/modoc/coco-shelf/gomar/ ; go build -o gomar0n --tags=noos,coco0 gomar.go )

    /home/strick/modoc/coco-shelf/gomar/gomar0n  \
        -ttl=600s \
        -write_rom_fail=1 \
        --entry=0x8000 -n=1 -raw_hyper_print=1   \
         -big_rom  moto.rom \
         -external_rom_listing   moto.rom.list
else
    test -s /home/strick/modoc/coco-shelf/gomar/gomar0nt || \
    ( cd /home/strick/modoc/coco-shelf/gomar/ ; go build -o gomar0nt --tags=noos,coco0,trace gomar.go )

    /home/strick/modoc/coco-shelf/gomar/gomar0nt  \
        -t=1  \
        -ttl=600s \
        -write_rom_fail=1 \
        --entry=0x8000 -n=1 -raw_hyper_print=1   \
         -big_rom  moto.rom \
         -external_rom_listing   moto.rom.list
fi
