#!/usr/bin/env python3
"""
MiniGolf Benchmark & Performance Comparison Framework (perf1)
Compares 4 M6809 C compilers on Hatvan VM:
  1. MiniGolf (C front end + M6809 backend)
  2. CMOC (-O2)
  3. GCC 6809 (-O2)
  4. GCC 6809 Max (-O2 -fomit-frame-pointer -fwhole-program)

Measures DECB payload size (bytes) and execution CPU cycles on Hatvan VM.
"""

import os
import sys
import re
import glob
import shutil
import argparse
import subprocess
from pathlib import Path

# Paths
PERF_DIR = Path(__file__).resolve().parent
REPO_ROOT = PERF_DIR.parent.parent
TESTS_DIR = PERF_DIR / "tests"
SUPPORT_DIR = PERF_DIR / "support"
BUILD_DIR = PERF_DIR / "build"
DISASM_DIR = PERF_DIR / "disasm"
MINIMAL_PRELUDE = REPO_ROOT / "c-demos" / "lnbasic" / "minimal"

# Tool paths
HATVAN_VM = REPO_ROOT.parent / "hatvan-os" / "hatvan-vm"
if not HATVAN_VM.exists():
    # Try finding in PATH
    which_vm = shutil.which("hatvan-vm")
    if which_vm:
        HATVAN_VM = Path(which_vm)

CMOC_LIB = Path("/home/strick/modoc/coco-shelf/share/cmoc/lib")
GCC_LIB = Path("/home/strick/modoc/coco-shelf/lib/gcc/m6809-unknown/4.6.4")

COMPILERS = ["minigolf", "cmoc", "gcc6809", "gcc6809max"]

def run_cmd(cmd, cwd=REPO_ROOT, check=True, input_str=None):
    """Run command and return CompletedProcess."""
    res = subprocess.run(
        cmd,
        cwd=cwd,
        shell=isinstance(cmd, str),
        text=True,
        input=input_str,
        capture_output=True,
    )
    if check and res.returncode != 0:
        raise RuntimeError(
            f"Command failed (exit {res.returncode}): {cmd}\n"
            f"STDOUT:\n{res.stdout}\n"
            f"STDERR:\n{res.stderr}"
        )
    return res

def parse_decb(path):
    """Parse DECB file and return metadata."""
    if not path.exists():
        return None
    data = path.read_bytes()
    pos = 0
    payload_size = 0
    min_addr = 0x10000
    max_addr = 0
    exec_addr = None

    while pos < len(data):
        chunk_type = data[pos]
        pos += 1
        if chunk_type == 0x00:
            if pos + 4 > len(data):
                break
            length = (data[pos] << 8) | data[pos + 1]
            addr = (data[pos + 2] << 8) | data[pos + 3]
            pos += 4
            if pos + length > len(data):
                break
            payload_size += length
            if addr < min_addr:
                min_addr = addr
            if addr + length > max_addr:
                max_addr = addr + length
            pos += length
        elif chunk_type == 0xFF:
            if pos + 4 <= len(data):
                exec_addr = (data[pos + 2] << 8) | data[pos + 3]
            break
        else:
            break

    if min_addr > max_addr:
        min_addr = max_addr = 0

    return {
        "file_size": len(data),
        "payload_size": payload_size,
        "load_range": (min_addr, max_addr),
        "exec_addr": exec_addr,
    }

def compile_minigolf(test_file, work_dir):
    """Compile test with MiniGolf C frontend -> DECB."""
    stem = test_file.stem
    core_asm = work_dir / f"{stem}_mg.core.asm"
    full_asm = work_dir / f"{stem}_mg.asm"
    decb_file = work_dir / f"{stem}_mg.decb"
    list_file = work_dir / f"{stem}_mg.list"
    map_file = work_dir / f"{stem}_mg.map"

    # Step 1: go run main.go -I ... -m=m6809
    cmd = [
        "go", "run", "main.go",
        "-I", str(PERF_DIR),
        "-I", str(MINIMAL_PRELUDE),
        "-m=m6809",
        f"-o={core_asm}",
        str(test_file)
    ]
    run_cmd(cmd, cwd=REPO_ROOT)

    # Step 2: Prepend cstart_minigolf.asm and append end cstart
    cstart_code = (SUPPORT_DIR / "cstart_minigolf.asm").read_text()
    core_code = core_asm.read_text()
    full_asm.write_text(cstart_code + "\n" + core_code + "\n    end cstart\n")

    # Step 3: lwasm -9 -b -f decb
    cmd = [
        "lwasm", "-9", "-b", "-f", "decb",
        f"--list={list_file}",
        f"--map={map_file}",
        "-o", str(decb_file),
        str(full_asm)
    ]
    run_cmd(cmd, cwd=work_dir)

    # Save to disasm
    test_disasm_dir = DISASM_DIR / stem
    test_disasm_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy2(full_asm, test_disasm_dir / "minigolf.asm")

    return decb_file

def compile_cmoc(test_file, work_dir):
    """Compile test with CMOC -O2 -> DECB."""
    stem = test_file.stem
    s_file = work_dir / f"{stem}_cmoc.s"
    o_file = work_dir / f"{stem}_cmoc.o"
    entry_o = work_dir / "cmoc_entry.o"
    decb_file = work_dir / f"{stem}_cmoc.decb"
    link_script = SUPPORT_DIR / "cmoc.link"

    # Step 1: cmoc -S -O2 -Wno-uncalled-static --org=8000 --intdir=work_dir
    cmd = [
        "cmoc", "-S", "-O2", "-Wno-uncalled-static", "--org=8000",
        f"--intdir={work_dir}",
        str(test_file)
    ]
    run_cmd(cmd, cwd=work_dir)
    src_s = work_dir / f"{test_file.name[:-2]}.s"
    if src_s.exists() and src_s != s_file:
        shutil.move(str(src_s), str(s_file))

    # Step 2: Assemble cmoc_entry.s
    cmd = [
        "lwasm", "-fobj",
        "-o", str(entry_o),
        str(SUPPORT_DIR / "cmoc_entry.s")
    ]
    run_cmd(cmd, cwd=work_dir)

    # Step 3: Assemble s_file
    cmd = [
        "lwasm", "-fobj", "--pragma=forwardrefmax", "-D_COCO_BASIC_",
        "-o", str(o_file),
        str(s_file)
    ]
    run_cmd(cmd, cwd=work_dir)

    # Step 4: Link with lwlink
    cmd = [
        "lwlink", "--format=decb",
        "-o", str(decb_file),
        "-s", str(link_script),
        str(entry_o),
        str(o_file),
        f"-L{CMOC_LIB}",
        "-lcmoc-crt-ecb",
        "-lcmoc-std-ecb",
        str(CMOC_LIB / "float-ctor.ecb_o"),
        "-lcmoc-float-ecb"
    ]
    run_cmd(cmd, cwd=work_dir)

    # Save to disasm
    test_disasm_dir = DISASM_DIR / stem
    test_disasm_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy2(s_file, test_disasm_dir / "cmoc.s")

    return decb_file

def compile_gcc(test_file, work_dir, is_max=False):
    """Compile test with GCC6809 or GCC6809max -> DECB."""
    stem = test_file.stem
    tag = "gcc6809max" if is_max else "gcc6809"
    s_file = work_dir / f"{stem}_{tag}.s"
    o_file = work_dir / f"{stem}_{tag}.o"
    support_o = work_dir / f"gcc_{tag}_support.o"
    decb_file = work_dir / f"{stem}_{tag}.decb"
    link_script = SUPPORT_DIR / "gcc.link"

    flags = ["-S", "-O2", "-fno-builtin"]
    if is_max:
        flags.extend(["-fomit-frame-pointer", "-fwhole-program"])

    # Step 1: gcc6809 flags test_file -o s_file
    cmd = ["gcc6809"] + flags + [str(test_file), "-o", str(s_file)]
    run_cmd(cmd, cwd=work_dir)

    # Step 2: Assemble gcc_support.s
    cmd = [
        "lwasm", "--obj",
        "--pragma=undefextern", "--pragma=cescapes",
        "--pragma=importundefexport", "--pragma=newsource",
        "-o", str(support_o),
        str(SUPPORT_DIR / "gcc_support.s")
    ]
    run_cmd(cmd, cwd=work_dir)

    # Step 3: Assemble s_file
    cmd = [
        "lwasm", "--obj",
        "--pragma=undefextern", "--pragma=cescapes",
        "--pragma=importundefexport", "--pragma=newsource",
        "-o", str(o_file),
        str(s_file)
    ]
    run_cmd(cmd, cwd=work_dir)

    # Step 4: Link with lwlink
    cmd = [
        "lwlink", "--format=decb",
        "-o", str(decb_file),
        "-s", str(link_script),
        f"-L{GCC_LIB}",
        "-lgcc",
        str(support_o),
        str(o_file)
    ]
    run_cmd(cmd, cwd=work_dir)

    # Save to disasm
    test_disasm_dir = DISASM_DIR / stem
    test_disasm_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy2(s_file, test_disasm_dir / f"{tag}.s")

    return decb_file

def run_hatvan_vm(decb_file):
    """Run DECB file on Hatvan VM and extract output and cycle count."""
    cmd = [str(HATVAN_VM), "--hypercalls", "--print-cycles", str(decb_file)]
    res = run_cmd(cmd, cwd=decb_file.parent, check=False)
    
    # Cycles printed on stderr or stdout: "[hatvan-vm finished: (\d+) total cycles executed]"
    combined = res.stdout + "\n" + res.stderr
    m = re.search(r"finished:\s*(\d+)\s*total cycles executed", combined)
    cycles = int(m.group(1)) if m else None

    # Normal program output is on stdout
    output = res.stdout
    return {
        "cycles": cycles,
        "output": output,
        "exit_code": res.returncode,
        "raw": combined
    }

def get_expected_output(test_file):
    """Compile and run test with host GCC to get authoritative expected output."""
    bin_file = BUILD_DIR / f"{test_file.stem}_host"
    cmd = ["gcc", "-Wall", "-Dunix=1", "-O2", str(test_file), "-o", str(bin_file)]
    run_cmd(cmd, cwd=REPO_ROOT)
    res = run_cmd([str(bin_file)], cwd=REPO_ROOT)
    return res.stdout

def run_test(test_file, compilers=None, verbose=False):
    """Run single benchmark across all requested compilers."""
    if compilers is None:
        compilers = COMPILERS

    stem = test_file.stem
    work_dir = BUILD_DIR / stem
    work_dir.mkdir(parents=True, exist_ok=True)

    expected_out = get_expected_output(test_file)
    results = {}

    for comp in compilers:
        try:
            if comp == "minigolf":
                decb = compile_minigolf(test_file, work_dir)
            elif comp == "cmoc":
                decb = compile_cmoc(test_file, work_dir)
            elif comp == "gcc6809":
                decb = compile_gcc(test_file, work_dir, is_max=False)
            elif comp == "gcc6809max":
                decb = compile_gcc(test_file, work_dir, is_max=True)
            else:
                raise ValueError(f"Unknown compiler: {comp}")

            decb_info = parse_decb(decb)
            vm_res = run_hatvan_vm(decb)
            
            # Check correctness
            verified = (vm_res["output"] == expected_out)

            results[comp] = {
                "status": "PASS" if verified else "FAIL_OUTPUT",
                "payload_size": decb_info["payload_size"],
                "file_size": decb_info["file_size"],
                "cycles": vm_res["cycles"],
                "output": vm_res["output"],
                "verified": verified
            }
            if verbose:
                print(f"  [{comp:11s}] payload={decb_info['payload_size']:4d} B | "
                      f"cycles={vm_res['cycles']:7d} | "
                      f"{'PASS' if verified else 'FAIL'}")
        except Exception as e:
            results[comp] = {
                "status": "ERROR",
                "error": str(e),
                "payload_size": 0,
                "file_size": 0,
                "cycles": 0,
                "verified": False
            }
            if verbose:
                print(f"  [{comp:11s}] ERROR: {e}")

    return results

def format_markdown_table(all_results):
    """Format benchmark results into Markdown tables."""
    lines = []
    lines.append("### Runtime Performance (CPU Cycles on Hatvan VM)")
    lines.append("")
    lines.append("| Benchmark Test | MiniGolf | CMOC (-O2) | GCC 6809 (-O2) | GCC6809 Max | MG vs GCC | MG vs GCCMax |")
    lines.append("| :--- | :---: | :---: | :---: | :---: | :---: | :---: |")

    for test_name, res in all_results.items():
        c_mg = res.get("minigolf", {}).get("cycles", 0)
        c_cmoc = res.get("cmoc", {}).get("cycles", 0)
        c_gcc = res.get("gcc6809", {}).get("cycles", 0)
        c_max = res.get("gcc6809max", {}).get("cycles", 0)

        r_gcc = f"{c_mg / c_gcc:.2f}x" if c_gcc else "N/A"
        r_max = f"{c_mg / c_max:.2f}x" if c_max else "N/A"

        lines.append(
            f"| `{test_name}` | **{c_mg:,}** | {c_cmoc:,} | {c_gcc:,} | {c_max:,} | {r_gcc} | {r_max} |"
        )

    lines.append("")
    lines.append("### Code Size (DECB Loaded Payload Bytes)")
    lines.append("")
    lines.append("| Benchmark Test | MiniGolf | CMOC (-O2) | GCC 6809 (-O2) | GCC6809 Max | MG vs GCC | MG vs GCCMax |")
    lines.append("| :--- | :---: | :---: | :---: | :---: | :---: | :---: |")

    for test_name, res in all_results.items():
        s_mg = res.get("minigolf", {}).get("payload_size", 0)
        s_cmoc = res.get("cmoc", {}).get("payload_size", 0)
        s_gcc = res.get("gcc6809", {}).get("payload_size", 0)
        s_max = res.get("gcc6809max", {}).get("payload_size", 0)

        r_gcc = f"{s_mg / s_gcc:.2f}x" if s_gcc else "N/A"
        r_max = f"{s_mg / s_max:.2f}x" if s_max else "N/A"

        lines.append(
            f"| `{test_name}` | **{s_mg} B** | {s_cmoc} B | {s_gcc} B | {s_max} B | {r_gcc} | {r_max} |"
        )

    return "\n".join(lines)

def generate_report(all_results, report_file):
    """Generate comprehensive README.md report."""
    md_tables = format_markdown_table(all_results)
    
    report = f"""# MiniGolf Performance Study: perf1

A comparative benchmark and code-generation study evaluating the **MiniGolf** compiler targeting the Motorola 6809 CPU against established C compilers:
- **MiniGolf**: Whole-program C front end & M6809 SSA optimization backend
- **CMOC 0.1.97**: Free 6809 C compiler targeting CoCo/Dragon bare-metal (`cmoc -S -O2 --org=8000`)
- **GCC 6809 4.6.4**: GCC port for 6809 with standard frame pointer (`gcc6809 -S -O2`)
- **GCC 6809 Max**: GCC 6809 with whole-program optimization and omitted frame pointers (`gcc6809 -S -O2 -fomit-frame-pointer -fwhole-program`)

All tests were executed bare-metal on the **Hatvan VM** (`hatvan-vm --hypercalls --print-cycles`) starting at address `$8000` with initial stack `S = $8000`.

---

## 1. Summary of Benchmark Results

{md_tables}

---

## 2. Test Descriptions & Benchmark Progression

The suite is structured in `perf/perf1/tests/` with increasing levels of compiler optimization stress:

1. **`01_putchar.c`**: Minimal baseline I/O poke (`putchar('@'); putchar('\\n');`). Tests function inlining and entry/exit overhead.
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
"""
    report_file.write_text(report)
    print(f"Report generated: {report_file}")

def main():
    parser = argparse.ArgumentParser(description="MiniGolf Performance Comparison Framework")
    subparsers = parser.add_subparsers(dest="command")

    # list
    subparsers.add_parser("list", help="List all benchmark tests")

    # run
    run_parser = subparsers.add_parser("run", help="Run benchmark tests")
    run_parser.add_argument("tests", nargs="*", help="Specific tests to run (default: all)")
    run_parser.add_argument("-c", "--compilers", nargs="+", choices=COMPILERS, default=COMPILERS,
                            help="Compilers to test")
    run_parser.add_argument("-v", "--verbose", action="store_true", help="Verbose output")
    run_parser.add_argument("--report", action="store_true", help="Generate README.md report after running")

    # report
    subparsers.add_parser("report", help="Generate README.md report from all tests")

    # clean
    subparsers.add_parser("clean", help="Clean build and disasm artifacts")

    args = parser.parse_args()

    if args.command == "list":
        for t in sorted(TESTS_DIR.glob("*.c")):
            print(t.stem)
        return

    if args.command == "clean":
        if BUILD_DIR.exists():
            shutil.rmtree(BUILD_DIR)
        if DISASM_DIR.exists():
            shutil.rmtree(DISASM_DIR)
        print("Cleaned build and disasm directories.")
        return

    # Default to run if no command given
    if args.command in ("run", "report", None):
        all_test_files = sorted(TESTS_DIR.glob("*.c"))
        if getattr(args, "tests", None):
            test_files = []
            for t in args.tests:
                f = TESTS_DIR / (t if t.endswith(".c") else f"{t}.c")
                if not f.exists():
                    # Try matching prefix
                    matches = list(TESTS_DIR.glob(f"*{t}*.c"))
                    if matches:
                        test_files.extend(matches)
                    else:
                        print(f"Error: test '{t}' not found.")
                        return
                else:
                    test_files.append(f)
        else:
            test_files = all_test_files

        compilers = getattr(args, "compilers", COMPILERS)
        verbose = getattr(args, "verbose", False)

        print(f"Running benchmarks on {len(test_files)} tests with compilers: {', '.join(compilers)}")
        print("=" * 70)

        all_results = {}
        for t in test_files:
            print(f"Testing {t.stem}...")
            res = run_test(t, compilers=compilers, verbose=verbose)
            all_results[t.stem] = res

        print("=" * 70)
        print(format_markdown_table(all_results))

        if getattr(args, "report", False) or args.command == "report":
            generate_report(all_results, PERF_DIR / "README.md")

if __name__ == "__main__":
    main()
