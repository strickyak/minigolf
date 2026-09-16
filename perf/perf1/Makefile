# MiniGolf Performance Comparison Framework (perf1)
# Compilers: MiniGolf vs CMOC (-O2) vs GCC6809 (-O2) vs GCC6809Max (-O2 -fomit-frame-pointer -fwhole-program)

PYTHON ?= python3
RUNNER = ./runner.py

all: test

test:
	$(PYTHON) $(RUNNER) run

test-verbose:
	$(PYTHON) $(RUNNER) run -v

report:
	$(PYTHON) $(RUNNER) report

compare:
	$(PYTHON) $(RUNNER) run

list:
	$(PYTHON) $(RUNNER) list

clean:
	$(PYTHON) $(RUNNER) clean
	rm -f *.decb *.s *.o *.asm *.list *.map

.PHONY: all test test-verbose report compare list clean
