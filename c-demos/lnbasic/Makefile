CC = gcc
CFLAGS = -std=c89 -pedantic -Wall -Werror -Dunix=1 -O2

HATVAN_VM = ../../../hatvan-os/hatvan-vm
CMOC = cmoc
GCC6809 = gcc6809
LWASM = lwasm
LWLINK = lwlink

CMOC_LIB = /home/strick/modoc/coco-shelf/share/cmoc/lib
GCC6809_LIB = /home/strick/modoc/coco-shelf/lib/gcc/m6809-unknown/4.6.4

TEST_INPUT = 10 REM Demo Program\n20 LET S = 0\n30 FOR I = 1 TO 5\n40 LET S = S + I\n50 PRINT "I="; I; ", SUM="; S\n60 NEXT I\n70 IF S = 15 THEN PRINT "VERIFIED!"\nLIST\nRUN\nBYE\n

all: lnbasic minigolf.decb cmoc.decb gcc6809.decb

# -------------------------------------------------------------
# 0. Host Linux GCC
# -------------------------------------------------------------
lnbasic: lnbasic.c
	$(CC) $(CFLAGS) lnbasic.c -o lnbasic

# -------------------------------------------------------------
# 1. MiniGolf with C Front End
# -------------------------------------------------------------
cstart_minigolf.asm:
	@printf '\tpragma cescapes\n\torg $$8000\ncstart:\n\tlds #$$8000\n\tclra\n\tclrb\n\ttfr d,x\n\ttfr d,y\n\ttfr d,u\n\tlbsr _main\n__exit0:\n\tclra\n\tclrb\n__exit:\n\tstb $$FF05\nstuck:\n\tbra stuck\n_printf:\n\trts\n' > $@

minigolf_core.asm: lnbasic.c
	cd ../.. && go run main.go -I c-demos/lnbasic/minimal -m=m6809 -o c-demos/lnbasic/minigolf_core.asm c-demos/lnbasic/lnbasic.c

minigolf.decb: cstart_minigolf.asm minigolf_core.asm
	cat cstart_minigolf.asm minigolf_core.asm > minigolf.asm
	echo "    end cstart" >> minigolf.asm
	$(LWASM) -9 -b -f decb --list=minigolf.list --map=minigolf.map -o minigolf.decb minigolf.asm

# -------------------------------------------------------------
# 2. CMOC (-S -O2)
# -------------------------------------------------------------
cmoc_entry.s:
	@printf '\tSECTION start\nmy_start EXPORT\n_main IMPORT\nmy_start:\n\tlds #$$8000\n\tlbsr _main\n\tclra\n\tclrb\n\tstb $$FF05\n.stuck:\n\tbra .stuck\n\tENDSECTION\n' > $@

cmoc.link:
	@printf 'define basesympat s_%%s\ndefine lensympat l_%%s\nsection start load 8000\nsection code\nsection constructors_start\nsection constructors\nsection constructors_end\nsection destructors_start\nsection destructors\nsection destructors_end\nsection initgl_start\nsection initgl\nsection initgl_end\nsection rodata\nsection rwdata\nsection bss,bss\nsection program_end\nentry my_start\n' > $@

cmoc.decb: cmoc_entry.s cmoc.link lnbasic.c
	$(CMOC) -S -O2 --org=8000 lnbasic.c
	mv lnbasic.s cmoc.s
	$(LWASM) -fobj --list=cmoc_entry.list -o cmoc_entry.o cmoc_entry.s
	$(LWASM) -fobj --pragma=forwardrefmax -D_COCO_BASIC_ --list=cmoc.list -o cmoc.o cmoc.s
	$(LWLINK) --format=decb --output=cmoc.decb --script=cmoc.link --map=cmoc.map cmoc_entry.o cmoc.o \
		-L$(CMOC_LIB) -lcmoc-crt-ecb -lcmoc-std-ecb $(CMOC_LIB)/float-ctor.ecb_o -lcmoc-float-ecb

# -------------------------------------------------------------
# 3. GCC 6809 (-S -O2)
# -------------------------------------------------------------
gcc6809_support.s:
	@printf '\t.module gcc6809_support.s\n\n\t.area .text.startup\n\t.globl cstart\n\t.globl _main\ncstart:\n\tlds #$$8000\n\tclra\n\tclrb\n\ttfr d,x\n\ttfr d,y\n\ttfr d,u\n\tlbsr _main\n\tclra\n\tclrb\n\tstb $$FF05\n.stuck:\n\tbra .stuck\n\n\t.area .text\n\t.globl _abort\n_abort:\n\tstb $$FF05\n.abort_stuck:\n\tbra .abort_stuck\n\n\t.globl _memcpy\n_memcpy:\n\tpshs y,u\n\tldy 6,s\n\tldd 8,s\n\tbeq .m_done\n\tleau ,x\n.m_loop:\n\tlda ,y+\n\tsta ,x+\n\tsubd #1\n\tbne .m_loop\n\ttfr u,x\n.m_done:\n\tpuls y,u,pc\n' > $@

gcc6809.link:
	@printf 'define basesympat s_%%s\ndefine lensympat l_%%s\nsection .text.startup load 8000\nsection .text\nsection .rodata\nsection .data\nsection .bss,bss\nentry cstart\n' > $@

gcc6809.decb: gcc6809_support.s gcc6809.link lnbasic.c
	$(GCC6809) -S -O2 lnbasic.c -o gcc6809.s
	$(LWASM) --obj --pragma=undefextern --pragma=cescapes --pragma=importundefexport --pragma=newsource --list=gcc6809_support.list -o gcc6809_support.o gcc6809_support.s
	$(LWASM) --obj --pragma=undefextern --pragma=cescapes --pragma=importundefexport --pragma=newsource --list=gcc6809.list -o gcc6809.o gcc6809.s
	$(LWLINK) --format=decb --output=gcc6809.decb --script=gcc6809.link --map=gcc6809.map -L$(GCC6809_LIB) -lgcc gcc6809_support.o gcc6809.o

# -------------------------------------------------------------
# Test Targets
# -------------------------------------------------------------
test: test-host test-minigolf test-cmoc test-gcc6809

test-host: lnbasic
	@echo "=== 0. Host GCC on Linux ==="
	@printf '$(TEST_INPUT)' | ./lnbasic

test-minigolf: minigolf.decb
	@echo "=== 1. MiniGolf C Front End on Hatvan VM ==="
	$(HATVAN_VM) --hypercalls --print-cycles --input='$(TEST_INPUT)' minigolf.decb

test-cmoc: cmoc.decb
	@echo "=== 2. CMOC (-S -O2) on Hatvan VM ==="
	$(HATVAN_VM) --hypercalls --print-cycles --input='$(TEST_INPUT)' cmoc.decb

test-gcc6809: gcc6809.decb
	@echo "=== 3. GCC 6809 (-S -O2) on Hatvan VM ==="
	$(HATVAN_VM) --hypercalls --print-cycles --input='$(TEST_INPUT)' gcc6809.decb

compare: all
	@echo "=========================================================="
	@echo "BINARY SIZE COMPARISON (DECB):"
	@echo "=========================================================="
	@ls -l minigolf.decb cmoc.decb gcc6809.decb
	@echo ""
	@echo "=========================================================="
	@echo "CYCLE COUNT COMPARISON ON HATVAN VM:"
	@echo "=========================================================="
	@$(MAKE) test-minigolf
	@$(MAKE) test-cmoc
	@$(MAKE) test-gcc6809

clean:
	rm -f lnbasic *.decb *.o *.s *.asm *.link *.list *.map
