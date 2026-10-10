MINIGOLF ?= ../minigolf
ASM6809 ?= ../asm6809
BIGGOLFLIB ?= ../biggolflib
COMPILE_BARE ?= ../compile-bare-metal.sh
COMPILE_HATVAN ?= ../compile-hatvan.sh
BUILD_DIR ?= build

SRC = pyeval.golf
DECB = $(BUILD_DIR)/pyeval.decb
DECB_HV = $(BUILD_DIR)/pyeval-hv.decb
TCL = $(BUILD_DIR)/mode81.tcl

.PHONY: all clean test run-gep9 run-tfr911

all: $(DECB) $(DECB_HV) $(TCL)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(DECB): $(SRC) $(COMPILE_BARE) ../bare-metal-lib/os.golf ../bare-metal-lib/cstart.asm | $(BUILD_DIR)
	$(COMPILE_BARE) $(SRC) $(DECB)

$(DECB_HV): $(SRC) $(COMPILE_HATVAN) ../hatvan-lib/os.golf ../hatvan-lib/cstart.asm | $(BUILD_DIR)
	$(COMPILE_HATVAN) $(SRC) $(DECB_HV)

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
