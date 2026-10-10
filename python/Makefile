MINIGOLF ?= ../minigolf
ASM6809 ?= ../asm6809
BIGGOLFLIB ?= ../biggolflib
BUILD_DIR ?= build

SRC = pyeval.golf
ASM = $(BUILD_DIR)/pyeval.asm
LIST = $(BUILD_DIR)/pyeval.list
DECB = $(BUILD_DIR)/pyeval.decb
TCL = $(BUILD_DIR)/mode81.tcl

.PHONY: all clean test run-gep9 run-tfr911

all: $(DECB) $(TCL)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(ASM): $(SRC) | $(BUILD_DIR)
	$(MINIGOLF) -m=6809+ -I=$(BIGGOLFLIB) -o=$@ $<

$(DECB): $(ASM) | $(BUILD_DIR)
	$(ASM6809) --decb -l $(LIST) -o $@ $<

$(TCL): | $(BUILD_DIR)
	@echo 'set Config(engine) "gep9big"' > $@
	@echo 'set Config(flat_img) "/pc/pyeval.decb"' >> $@
	@echo 'set Config(task1) ""' >> $@
	@echo 'set Config(task2) ""' >> $@
	@echo 'set Config(drive0) ""' >> $@
	@echo 'set Config(drive1) ""' >> $@
	@echo 'set Config(drive2) ""' >> $@
	@echo 'set Config(drive3) ""' >> $@
	@echo 'menu store Config' >> $@
	@echo 'bye' >> $@

clean:
	rm -rf $(BUILD_DIR)

test: all
	./test_pyeval.sh

run-gep9: all
	./run_py_gep9.sh

run-tfr911: all
	./run_py_tfr911.sh
