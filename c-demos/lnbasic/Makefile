CC = gcc
CFLAGS = -std=c89 -pedantic -Wall -Werror -Dunix=1 -O2
MINIGOLF = ../../minigolf
HATVAN_VM = ../../../hatvan-os/hatvan-vm

all: lnbasic

lnbasic: lnbasic.c
	$(CC) $(CFLAGS) lnbasic.c -o lnbasic

cstart.asm:
	@printf '\tpragma cescapes\n\torg $$8000\ncstart:\n\tlds #$$8000\n\tclra\n\tclrb\n\ttfr d,x\n\ttfr d,y\n\ttfr d,u\n\tlbsr _main\n__exit0:\n\tclra\n\tclrb\n\ttfr d,x\n__exit:\n\tfcb $$12,$$21,107\n\tstb $$FF05\nstuck:\n\tbra stuck\n_printf:\n\trts\n' > cstart.asm

lnbasic.asm: lnbasic.c
	cd ../.. && go run main.go -I=golflib -I=c-tests -m=m6809 -o c-demos/lnbasic/lnbasic_core.asm c-demos/lnbasic/lnbasic.c

lnbasic.decb: cstart.asm lnbasic.asm
	cat cstart.asm lnbasic_core.asm > lnbasic.asm
	echo "    end cstart" >> lnbasic.asm
	lwasm -9 -b -f decb -o lnbasic.decb lnbasic.asm

test: lnbasic
	@echo "=== Testing Line Number BASIC with GCC ==="
	@printf '10 REM Demo Program\n20 LET S = 0\n30 FOR I = 1 TO 5\n40 LET S = S + I\n50 PRINT "I="; I; ", SUM="; S\n60 NEXT I\n70 IF S = 15 THEN PRINT "VERIFIED!"\nLIST\nRUN\nBYE\n' | ./lnbasic
	@if [ -x $(HATVAN_VM) ]; then \
		echo "=== Testing Line Number BASIC on M6809 (hatvan-vm) ==="; \
		$(MAKE) lnbasic.decb && \
		$(HATVAN_VM) --input='10 REM Demo Program\n20 LET S = 0\n30 FOR I = 1 TO 5\n40 LET S = S + I\n50 PRINT "I="; I; ", SUM="; S\n60 NEXT I\n70 IF S = 15 THEN PRINT "VERIFIED!"\nLIST\nRUN\nBYE\n' lnbasic.decb; \
	fi

clean:
	rm -f lnbasic cstart.asm lnbasic_core.asm lnbasic.asm *.o *.decb *.list *.map
