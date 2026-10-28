# How to Bootstrap the Par3 Compiler (`minigolf_par3`)

The Par3 compiler ([`par3-apps/par3comp.par3.and.go`](../par3-apps/par3comp.par3.and.go)) is written in the **Par3** subset of Go so that it is simultaneously:
1. **Valid standard Go** when compiled alongside [`par3-apps/par3comp_gosupport.go`](../par3-apps/par3comp_gosupport.go) on a host machine.
2. **Self-hosting Par3** when compiled by itself (without `par3comp_gosupport.go`), where the support primitives (`mk_str`, `str_base`, `free_str`, `get_arg`, `file_*`, `map_*`, `sys_*`) are emitted directly as VM bytecode instructions executed by [`par3-runtime/par3runtime.asm`](../par3-runtime/par3runtime.asm) (or [`par3-runtime/par3vm.py`](../par3-runtime/par3vm.py)).

Below are step-by-step shell commands you can run from the repository root to watch the bootstrap and self-compilation converge byte-for-byte.

---

## 1. Stage 0: Build the Bootstrap Compiler with Go

Compile [`par3-apps/par3comp.par3.and.go`](../par3-apps/par3comp.par3.and.go) and [`par3-apps/par3comp_gosupport.go`](../par3-apps/par3comp_gosupport.go) using the standard Go toolchain:

```bash
mkdir -p _tmp
go build -o _tmp/minigolf_par3_stage0 \
    par3-apps/par3comp.par3.and.go \
    par3-apps/par3comp_gosupport.go
```

---

## 2. Stage 1: Compile `par3comp.par3.and.go` and `par3asm.par3` to P-Code

Use the Go-built `_tmp/minigolf_par3_stage0` binary to compile [`par3-apps/par3comp.par3.and.go`](../par3-apps/par3comp.par3.and.go) (notice `par3comp_gosupport.go` is **not** passed here) and the Par3 bytecode assembler ([`par3-apps/par3asm.par3`](../par3-apps/par3asm.par3)) into `.p3a` assembly, then assemble them into `.p3p` bytecode binaries:

```bash
# Compile Par3 source -> .p3a P-code assembly (CR line endings for NitrOS-9)
./_tmp/minigolf_par3_stage0 par3-apps/par3comp.par3.and.go -o _tmp/minigolf_par3.stage1.p3a
./_tmp/minigolf_par3_stage0 par3-apps/par3asm.par3         -o _tmp/par3asm.stage1.p3a

# Convert CR -> LF for the Python bootstrap assembler and assemble -> .p3p bytecode
tr '\r' '\n' < _tmp/minigolf_par3.stage1.p3a > _tmp/minigolf_par3.stage1.lf.p3a
tr '\r' '\n' < _tmp/par3asm.stage1.p3a       > _tmp/par3asm.stage1.lf.p3a

python3 par3-runtime/par3asm.py _tmp/minigolf_par3.stage1.lf.p3a -o _tmp/minigolf_par3.stage1.p3p
python3 par3-runtime/par3asm.py _tmp/par3asm.stage1.lf.p3a       -o _tmp/par3asm.stage1.p3p
```

---

## 3. Stage 2: Self-Compile and Assemble on the Par3 VM

Now run the Stage 1 `.p3p` compiler and assembler bytecodes inside the Par3 VM ([`par3-runtime/par3vm.py`](../par3-runtime/par3vm.py), which simulates the 64 KB process memory space) to compile and assemble `par3comp.par3.and.go` a second time, and verify with `cmp` that Stage 1 and Stage 2 are identical:

```bash
# Self-compile par3comp.par3.and.go using the Stage 1 Par3 bytecode compiler
python3 par3-runtime/par3vm.py _tmp/minigolf_par3.stage1.p3p -- \
    par3-apps/par3comp.par3.and.go -o _tmp/minigolf_par3.stage2.p3a

# Verify Stage 1 and Stage 2 .p3a assembly outputs are byte-for-byte identical
cmp _tmp/minigolf_par3.stage1.p3a _tmp/minigolf_par3.stage2.p3a && echo "Stage 1 == Stage 2 (.p3a)"

# Assemble the Stage 2 .p3a using the Stage 1 par3asm.p3p bytecode assembler
python3 par3-runtime/par3vm.py _tmp/par3asm.stage1.p3p -- \
    _tmp/minigolf_par3.stage2.p3a -o _tmp/minigolf_par3.stage2.p3p

# Verify Stage 1 and Stage 2 .p3p bytecode binaries are byte-for-byte identical
cmp _tmp/minigolf_par3.stage1.p3p _tmp/minigolf_par3.stage2.p3p && echo "Stage 1 == Stage 2 (.p3p)"
```

You can also run the automated Go test that performs Steps 1–3 across all five tools in `par3-apps/`:

```bash
go test -v -run TestPar3GoBootstrapAndSelfCompile .
```

---

## 4. Stage 3: Build Native M6809 NitrOS-9 Modules (`par3runtime.asm`)

To package any `.p3p` bytecode file with the Motorola 6809 VM runtime ([`par3-runtime/par3runtime.asm`](../par3-runtime/par3runtime.asm)) into a standalone NitrOS-9 Level 2 executable module:

1. **On the M6809 under NitrOS-9 Level 2 (64 KB process space)**:
   ```text
   minigolf_par3 /dd/SRC/par3comp.par3.and.go /dd/Red/minigolf_par3.p3a
   par3asm /dd/Red/minigolf_par3.p3a -o /dd/Red/minigolf_par3.p3p
   par3pack /dd/Red/minigolf_par3.p3p minigolf_par3 /dd/Red
   chd /dd/Red
   asm #48k par3runtime.prep.asm u -o=/dd/Red/CMDS/minigolf_par3
   attr /dd/Red/CMDS/minigolf_par3 e pe
   ```

2. **Full 3-Stage Self-Regeneration on `gep9` (Red $\rightarrow$ Green $\rightarrow$ Blue)**:
   To boot NitrOS-9 Level 2 (`deep65280`) in the `gep9` emulator and watch the M6809 rebuild the entire NitrOS-9 kernel, boot image, and all 5 Par3 tools (`minigolf_par3`, `par3asm`, `par3pack`, `selfgen_preprocess`, `build_img`) through three generations ending with `cmp` verification:
   ```bash
   ./par3-apps/selfgen-on-gep9.sh
   ```
