set -ex

case "$1" in 
    *.golf )
        go run main.go -m M6809 -o _tmp/main.asm  -I golflib  "$1" >&2
        ;;
    *.s | *.asm )
        cp -fv "$1" _tmp/main.asm >&2
        ;;
    * )
        echo "BAD EXTENSION: Expected .golf or .s or .asm: '$1'" >&2
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

_printf:
    leax 2,s
    fcb  $12,$21,111  ; Hyper Printf
    rts

f_prelude.mul_byte:
    ; first byte arg is already in B
    lda 2,s   ; get second byte arg
    mul
    tfr d,x   ; leave result in X
    rts



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

test -s /home/strick/modoc/coco-shelf/gomar/gomar0n || \
( cd /home/strick/modoc/coco-shelf/gomar/ ; go build -o gomar0n --tags=noos,coco0,trace gomar.go )

/home/strick/modoc/coco-shelf/gomar/gomar0n  \
        -ttl=10s \
        -write_rom_fail=1 -t=1 --entry=0x8000 -n=1 -raw_hyper_print=1   \
         -big_rom  /home/strick/antig/_tmp/moto.rom \
         -external_rom_listing   /home/strick/antig/_tmp/moto.rom.list
