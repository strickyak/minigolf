set -ex
T=/tmp/run-6809.$(echo "$1" | tr -c A-Za-z0-9 _).tmp

rm -rf $T
mkdir -p $T
cp -f "$1" $T/main.asm

cd $T

cat >script <<HERE
section .bss           load 0x0A00
section .data
section .entry         load 0x7F00
section .text          load 0x4000
section code           load 0x4000
section *
HERE

cat >cstart.asm <<'HERE'
	pragma cescapes
	pragma undefextern
    pragma undefextern
    pragma importundefexport

    .area .entry
entry:
    lds #$4000
    clra
    clrb
    tfr d,x
    tfr d,y
    tfr d,u
    lbsr _main
    clra
    clrb
    tfr d,x
    fcb  $12,$21,107  ; Hyper Exit with 0.
stuck: bra stuck
    export entry

_printf:
    leax 2,s
    fcb  $12,$21,111  ; Hyper Printf
    rts
    export _printf

HERE

- lwasm --format=obj -o'main.o' --pragma=undefextern  main.asm
- lwasm --format=obj -o'cstart.o' cstart.asm
- lwlink --format=decb -o'main.decb' --script=script --entry=entry main.o cstart.o

( cd /home/strick/modoc/coco-shelf/gomar/ && go build --tags=level1,coco1,trace gomar.go )

/home/strick/modoc/coco-shelf/gomar/gomar \
    --loadm 'main.decb' \
    --entry=0x7F00 \
    -t=1 \
    -n=1 \
    -raw_hyper_print=1 \
    | tee _out
Z=$?

sleep 2

case $Z in
    0) : okay ;;
    *) echo "EXITED STATUS $Z" >&2 ; exit $Z ;;
esac
