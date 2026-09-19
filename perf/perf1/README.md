# MiniGolf Performance Study: perf1

A comparative benchmark and code-generation study evaluating the **MiniGolf** compiler targeting the Motorola 6809 CPU against established C compilers:
- **MiniGolf**: Whole-program C front end & M6809 SSA optimization backend
- **CMOC 0.1.97**: Free 6809 C compiler targeting CoCo/Dragon bare-metal (`cmoc -S -O2 --org=8000`)
- **GCC 6809 4.6.4**: GCC port for 6809 with standard frame pointer (`gcc6809 -S -O2`)
- **GCC 6809 Max**: GCC 6809 with whole-program optimization and omitted frame pointers (`gcc6809 -S -O2 -fomit-frame-pointer -fwhole-program`)

All tests were executed bare-metal on the **Hatvan VM** (`hatvan-vm --hypercalls --print-cycles`) starting at address `$8000` with initial stack `S = $8000`.

---

## 1. Summary of Benchmark Results

### Runtime Performance (CPU Cycles on Hatvan VM)

| Benchmark Test | MiniGolf | CMOC (-O2) | GCC 6809 (-O2) | GCC6809 Max | MG vs CMOC | MG vs GCC | MG vs GCCMax |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `01_putchar` | **100** | 122 | 75 | 66 | 0.82x | 1.33x | 1.52x |
| `02_count_loop` | **849** | 895 | 542 | 531 | 0.95x | 1.57x | 1.60x |
| `03_arithmetic` | **14,280** | 14,679 | 16,390 | 16,409 | 0.97x | 0.87x | 0.87x |
| `04_fibonacci` | **17,874** | 14,322 | 12,466 | 11,519 | 1.25x | 1.43x | 1.55x |
| `05_array_sum` | **13,987** | 12,165 | 11,997 | 11,836 | 1.15x | 1.17x | 1.18x |
| `06_string_ops` | **9,627** | 7,271 | 5,643 | 5,319 | 1.32x | 1.71x | 1.81x |
| `07_sieve` | **23,982** | 17,772 | 14,256 | 14,800 | 1.35x | 1.68x | 1.62x |
| `08_bubble_sort` | **52,876** | 44,102 | 42,391 | 41,207 | 1.20x | 1.25x | 1.28x |
| `09_struct_ops` | **9,642** | 8,173 | 8,015 | 7,891 | 1.18x | 1.20x | 1.22x |
| `10_switch_case` | **9,132** | 6,808 | 6,865 | 6,794 | 1.34x | 1.33x | 1.34x |
| **Min** | - | - | - | - | 0.82x | 0.87x | 0.87x |
| **Max** | - | - | - | - | 1.35x | 1.71x | 1.81x |
| **Mean** | - | - | - | - | 1.15x | 1.35x | 1.40x |

### Code Size (DECB Loaded Payload Bytes)

| Benchmark Test | MiniGolf | CMOC (-O2) | GCC 6809 (-O2) | GCC6809 Max | MG vs CMOC | MG vs GCC | MG vs GCCMax |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `01_putchar` | **47 B** | 204 B | 92 B | 87 B | 0.23x | 0.51x | 0.54x |
| `02_count_loop` | **99 B** | 251 B | 139 B | 133 B | 0.39x | 0.71x | 0.74x |
| `03_arithmetic` | **649 B** | 659 B | 586 B | 548 B | 0.98x | 1.11x | 1.18x |
| `04_fibonacci` | **516 B** | 459 B | 362 B | 333 B | 1.12x | 1.43x | 1.55x |
| `05_array_sum` | **718 B** | 571 B | 373 B | 348 B | 1.26x | 1.92x | 2.06x |
| `06_string_ops` | **844 B** | 710 B | 514 B | 466 B | 1.19x | 1.64x | 1.81x |
| `07_sieve` | **598 B** | 537 B | 354 B | 329 B | 1.11x | 1.69x | 1.82x |
| `08_bubble_sort` | **888 B** | 648 B | 427 B | 395 B | 1.37x | 2.08x | 2.25x |
| `09_struct_ops` | **1046 B** | 653 B | 326 B | 303 B | 1.60x | 3.21x | 3.45x |
| `10_switch_case` | **804 B** | 737 B | 593 B | 569 B | 1.09x | 1.36x | 1.41x |
| **Min** | - | - | - | - | 0.23x | 0.51x | 0.54x |
| **Max** | - | - | - | - | 1.60x | 3.21x | 3.45x |
| **Mean** | - | - | - | - | 1.04x | 1.57x | 1.68x |

---

## 2. Test Descriptions & Benchmark Progression

The suite is structured in `perf/perf1/tests/` with increasing levels of compiler optimization stress:

1. **`01_putchar.c`**: Minimal baseline I/O poke (`putchar('@'); putchar('\n');`). Tests function inlining and entry/exit overhead.
2. **`02_count_loop.c`**: 10-iteration loop printing `'0'..'9'`. Tests loop induction, branch tests, and register reuse.
3. **`03_arithmetic.c`**: Evaluates signed 16-bit math `(a*b+c)/d - (a%b)` in a loop with dynamic inputs. Tests 16-bit multiply, divide, modulus, and register pressure.
4. **`04_fibonacci.c`**: Recursive calculation of `fib(10) = 55`. Stresses function call overhead, stack frame allocation/deallocation (`leas` vs `pshs/puls`), and recursion.
5. **`05_array_sum.c`**: Summing a 10-element integer array using both indexed access `arr[i]` and pointer traversal `*p++`. Tests 6809 addressing modes (indexed `,x` vs autoincrement `,x+`).
6. **`06_string_ops.c`**: String operations: custom `strlen`, `strcpy`, and `strcmp`. Stresses byte-level operations, null byte scanning, and 8-bit accumulator (`A`/`B`) usage.
7. **`07_sieve.c`**: Sieve of Eratosthenes finding 25 primes up to 100. Classic 8-bit benchmark stressing byte array random access and nested loops.
8. **`08_bubble_sort.c`**: In-place bubble sort of 10 integers. Stresses nested loops, array element swaps, and read-modify-write patterns.
9. **`09_struct_ops.c`**: Nested struct field access (`Rect` and `Point`), computing rectangle area and point containment. Tests struct layout and offset calculations.
10. **`10_switch_case.c`**: Mini virtual machine / bytecode dispatch executing 8 instructions via `switch(op)`. Compares jump tables vs if-else cascade implementations.

---

## 3. Key Observations & Code Generation Differences

By inspecting the disassembled assembly in `perf/perf1/disasm/`, key differences explain the cycle count and code size ratios:

### A. Stack Frame Allocation and Local Variable Spilling
- **GCC / GCC Max**: Leverages 6809 registers (`X`, `U`, `Y`, `D`) for local variables and loop counters whenever possible. Leaves leaf functions with 0 stack allocation.
- **MiniGolf**: Generates fixed stack frames (e.g. `leas -23,s` or `leas -57,s`) and repeatedly spills/reloads temporaries from the stack on almost every operation.
  - *Example in loop*: MiniGolf frequently performs `leax 0,s; tfr x,d; tfr d,u; ldd ,u; std 2,s` taking ~20 cycles just to fetch a variable from the stack into `D`.

### B. Addressing Mode Exploitation
- **GCC**: Heavily uses autoincrement `,x+` and `,y+` for pointer loops, reducing both instruction bytes and execution cycles.
- **MiniGolf**: Emits explicit pointer arithmetic: `ldd ptr; addd #1; std ptr; ldb ,x`.

### C. Calling Conventions and Leaf Functions
- **GCC Max**: Omits frame pointers (`-fomit-frame-pointer`). Leaf functions do not push/pull registers or touch `S` unless needed.
- **MiniGolf**: Uses `jsr` and sets up a new frame even for minimal helper functions, adding function prologue/epilogue overhead.

### D. Switch / Case Statements
- **GCC / CMOC**: Implements dense switch statements using 6809 indexed jump tables (`jmp [d,x]`), executing in O(1) time regardless of case count.
- **MiniGolf**: MiniGolf translates `switch` into cascaded `if-else` chains in `ctranslator`, requiring O(N) comparisons and conditional branches.

---

## 4. Prioritized Optimization Roadmap for MiniGolf

Based on these empirical results, the following optimizations will yield the highest performance gains for MiniGolf:

1. **Redundant Spill/Reload Elimination**: Remove consecutive `std X,s; ldd X,s` or `tfr x,d; tfr d,u; ldd ,u`.
2. **Direct Stack Addressing**: Replace `leax offset,s; ldd ,x` with direct `ldd offset,s`.
3. **Register Allocation for Loop Variables**: Keep loop induction variables in index registers (`X` or `Y`) across iterations instead of reloading from stack slots.
4. **Autoincrement/Autodecrement Addressing**: Pattern-match pointer increment loops to emit `lda ,x+` / `sta ,x+`.
5. **Jump Tables for Dense Switches**: Support jump table dispatch for dense integer switch ranges.

---

## 5. How to Run the Benchmark Suite

```bash
# Run all tests and print comparison table
make test

# Generate this report
make report

# Run a specific test
python3 runner.py run 04_fibonacci
```
