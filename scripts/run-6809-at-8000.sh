set -ex
T=$PWD/_tmp/run-6809.$(echo "$1" | tr -c A-Za-z0-9 _).tmp

rm -rf $T
mkdir -p $T
cp -f "$1" $T/main.asm

cd $T

cat >script <<HERE
section .bss           load 0x0A00
section .data
section .entry         load 0x8000
section .text          load 0x8100
section code           load 0x8100
section *
HERE

cat >cstart.asm <<'HERE'
	pragma cescapes
	pragma undefextern
    pragma undefextern
    pragma importundefexport

    .area .entry
entry:
    lds #$8100
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

    fill 1,0x8100-*   ; pad with illegals until next section

HERE

time - lwasm --format=obj -o'main.o' --pragma=undefextern  main.asm
time - lwasm --format=obj -o'cstart.o' cstart.asm
: time - lwlink --format=decb -o'main.decb' --script=script --entry=entry main.o cstart.o
time - lwlink --format=raw -o'main.rom' --script=script --entry=entry main.o cstart.o

# ( cd /home/strick/modoc/coco-shelf/gomar/ && go build --tags=level1,coco1,trace gomar.go )
( cd /home/strick/modoc/coco-shelf/gomar/ && time go build --tags=level1,coco1 gomar.go )

time /home/strick/modoc/coco-shelf/gomar/gomar \
    --loadm 'main.decb' \
    --entry=0x7F00 \
    -n=1 \
    -raw_hyper_print=1 \
    | tee _out
Z=$?
    # -t=1 \

case $Z in
    0) : okay ;;
    *) echo "EXITED STATUS $Z" >&2 ; exit $Z ;;
esac
