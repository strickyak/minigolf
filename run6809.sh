set -ex

go run main.go -m M6809 -o _tmp/main.asm  -I golflib  "$1"

# sh  scripts/run-6809-at-8000.sh  _tmp/m.s

cd _tmp

cat >script <<HERE
section .bss           load 0x0A00
section bss
section .data
section data
section .entry         load 0x8000
section .text          load 0x8100
section text
section .code
section code
section *
HERE

cat >cstart.asm <<'HERE'
	pragma cescapes
	pragma undefextern
    pragma undefextern
    pragma importundefexport

    .area .entry
entry:
    daa
    daa
    daa
    daa
    daa
    daa
    daa
    daa

    lds  #$8000
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
    export entry
    export __exit
    export __exit0

_printf:
    leax 2,s
    fcb  $12,$21,111  ; Hyper Printf
    rts
    export _printf

    fill 1,0x0100-*   ; pad with illegals until next section

HERE

time - lwasm --format=obj -o'main.o' --pragma=undefextern  main.asm
time - lwasm --format=obj -o'cstart.o' cstart.asm
time - lwlink --format=raw -o'main.rom' --script=script --entry=entry main.o cstart.o

#############

( cd /home/strick/modoc/coco-shelf/gomar/ ; go build --tags=level1,coco1,trace gomar.go )

/home/strick/modoc/coco-shelf/gomar/gomar  -write_rom_fail=1 -t=1 --entry=0x8000 -n=1 -raw_hyper_print=1   -rom_8000  /home/strick/antig/_tmp/main.rom 

