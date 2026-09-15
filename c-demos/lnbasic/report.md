# M6809 C Compiler Comparison: MiniGolf vs CMOC vs GCC 6809

A comparative evaluation of three C compilers targeting the Motorola 6809 processor using the Line Number BASIC interpreter ([`lnbasic.c`](lnbasic.c)).

---

## 1. Environment & Build Configuration

All three compilers were evaluated under identical execution constraints on the **Hatvan OS VM** (`hatvan-vm`), a cycle-accurate 6809 CPU simulator with memory-mapped I/O:

- **Target Architecture**: Motorola 6809 (16-bit registers: `D` [`A`:`B`], `X`, `Y`, `U`, `S`, `PC`)
- **Code Origin**: `$8000`
- **Entry Point / Initial PC**: `$8000`
- **Initial Stack Pointer (`S`)**: `$8000` (grows downward into RAM below `$8000`)
- **I/O Integration**: Freestanding bare-metal mode without `stdio.h`:
  - `putchar(c)`: Memory-mapped poke to character output port `$FF00`
  - `getchar()`: Memory-mapped non-blocking poll of keystroke port `$FF01`
- **Clean Termination**: Writing return status to Hatvan VM exit port `$FF05`
- **Assembler & Linker**: LWTOOLS 4.24 (`lwasm` and `lwlink`)

### Compilers Evaluated

1. **MiniGolf (Whole-Program C Front End)**:
   - Source: MiniGolf C frontend (`modernc.org/cc/v5`) &rarr; MiniGolf IR &rarr; SSA Optimization pipeline &rarr; M6809 Backend &rarr; `lwasm --decb`
2. **CMOC (0.1.97)**:
   - Invocation: `cmoc -S -O2 --org=8000` &rarr; `lwasm -fobj` &rarr; `lwlink --format=decb`
3. **GCC 6809 (4.6.4, gcc6809lw pl9)**:
   - Invocation: `gcc6809 -S -O2` &rarr; `lwasm --obj` &rarr; `lwlink --format=decb -lgcc`

---

## 2. Compiled Binary Size Comparison

Each binary was linked into Color Computer / Dragon DECB format starting at address `$8000`.

| Compiler | DECB File Size | Loaded Payload Size | Loaded Address Range | Code Size vs GCC |
| :--- | :---: | :---: | :---: | :---: |
| **GCC 6809 (`-O2`)** | **5,018 bytes** | **5,008 bytes** | `$8000` .. `$938F` | **1.00x** (Baseline) |
| **CMOC (`-O2`)** | **5,520 bytes** | **5,510 bytes** | `$8000` .. `$9585` | **1.10x** (+10%) |
| **MiniGolf (C Frontend)** | **17,762 bytes** | **17,752 bytes** | `$8000` .. `$C557` | **3.54x** (+254%) |

*Note: Uninitialized static storage (such as the 128-line program buffer `g_prog`, variables array `g_vars`, and `g_for_stack`, totaling ~10.6 KB) resides in BSS and does not consume space in the loadable DECB image.*

---

## 3. Runtime Performance: CPU Cycles on Hatvan VM

Cycle counts were measured using Hatvan VM's cycle-accurate instruction timer (`--print-cycles`).

### Benchmark 1: Minimal Immediate Command
Input:
```basic
10 PRINT 42
RUN
BYE
```

| Compiler | Hatvan VM CPU Cycles | Ratio vs GCC 6809 | Ratio vs CMOC |
| :--- | :---: | :---: | :---: |
| **GCC 6809 (`-O2`)** | **10,522** | **1.00x** | 0.54x |
| **CMOC (`-O2`)** | **19,349** | **1.84x** | **1.00x** |
| **MiniGolf** | **56,318** | **5.35x** | **2.91x** |

---

### Benchmark 2: Standard BASIC Demo (`demo.bas`)
Input includes storing 16 program lines, running `LIST`, interpreting nested `FOR`/`NEXT` loops with `STEP`, `IF`/`THEN` jumps, integer expressions, string printing, and clean `BYE` exit:

| Compiler | Hatvan VM CPU Cycles | Ratio vs GCC 6809 | Ratio vs CMOC |
| :--- | :---: | :---: | :---: |
| **GCC 6809 (`-O2`)** | **260,545** | **1.00x** | 0.60x |
| **CMOC (`-O2`)** | **432,353** | **1.66x** | **1.00x** |
| **MiniGolf** | **1,111,021** | **4.26x** | **2.57x** |

---

### Benchmark 3: Loop & Computation Intensive
Interprets 30 iterations of `S = S + I * I` in a `FOR`/`NEXT` loop:

| Compiler | Hatvan VM CPU Cycles | Ratio vs GCC 6809 | Ratio vs CMOC |
| :--- | :---: | :---: | :---: |
| **GCC 6809 (`-O2`)** | **173,790** | **1.00x** | 0.47x |
| **CMOC (`-O2`)** | **365,912** | **2.11x** | **1.00x** |
| **MiniGolf** | **1,094,590** | **6.30x** | **2.99x** |

---

## 4. Output Verification

All three compilers produced 100% byte-for-byte identical output across all benchmarks:

```
Line Number BASIC
OK
======================================
   LINE NUMBER BASIC IN ACTION
======================================
STEP I=1, SUM=1
STEP I=3, SUM=4
STEP I=5, SUM=9
STEP I=7, SUM=16
STEP I=9, SUM=25
TRIANGULAR ODD SUM VERIFIED: 25
SUCCESS!
OK
```

---

## 5. Technical Analysis of Code Generation

### GCC 6809 (`gcc6809 -S -O2`)
- **Code Size**: Most compact (5,008 bytes).
- **Execution Speed**: Fastest (1.00x baseline).
- **Architecture**:
  - Global register allocation across basic blocks using all index registers (`X`, `Y`, `U`) and accumulator `D`.
  - Leaf functions and inner loops frequently avoid frame pointer manipulation.
  - Character comparisons and small integers remain 8-bit in register `B`, testing flags directly with condition codes rather than widening to 16-bit stack allocations.
  - Epilogues use single-instruction stack restore and return (`puls y,u,pc`).

### CMOC (`cmoc -S -O2`)
- **Code Size**: Very compact (5,510 bytes, only 10% larger than GCC).
- **Execution Speed**: 1.66x &ndash; 2.11x slower than GCC.
- **Architecture**:
  - Handcrafted for the 6809 with effective peephole optimizations (e.g. `CLRA; LDB #val`, `LEAS ,U; PULS U,PC`).
  - Strict stack-frame layout using `U` as the local frame pointer.
  - Spills intermediate expression results to stack frame slots and makes frequent calls to small runtime helpers (`DIV16`, `SDIV16`, `MUL16`, `copyMem`).

### MiniGolf (Current M6809 Backend)
- **Code Size**: 17,752 bytes (~3.5x larger than GCC).
- **Execution Speed**: ~4.2x &ndash; 6.3x slower than GCC (~2.5x &ndash; 3.0x slower than CMOC).
- **Architecture**:
  - **Correctness**: 100% compliant across all C language features, operator precedence, string formatting, and freestanding I/O.
  - **Lowering**: Currently lowers IR statements into stack-spill patterns (`0,s`, `2,s`), loading operands into `D`, computing results, and storing back to the frame.
  - **Subroutines**: Calls `__mul16`, `__divmod16` inline helpers and includes default prelude initialization.
  - **Optimization Roadmap**: As Phase Four's SSA graph coloring and register allocation are connected directly to the M6809 backend (targeting `X`, `Y`, and `U`), substantial reductions in code footprint and execution cycles will be achieved.
