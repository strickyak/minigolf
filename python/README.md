# Python Expression Evaluator for MiniGolf (big6809)

An interactive Python expression evaluator and REPL written in the **MiniGolf** language, compiled for the **EMBIGGEN 6809** target platform (`-m=6809+`) and executable either in the Hatvan OS `gep9` virtual machine or directly on the physical **TFR911H** 6309 hardware board with the `ENGINE_GEP9BIG` engine.

## Features

- **Prompting REPL**: Interactive `>>> ` prompt evaluating expressions and assignments line by line.
- **Global Variable Assignment**:
  - `var = expr` (e.g., `x = 100`)
  - `lst[i] = expr` (e.g., `a[0] = 99`)
  - `dict[key] = expr` (e.g., `d['arch'] = 6809`)
- **Data Types**:
  - 16-bit signed integers (`int`)
  - Booleans (`True`, `False`)
  - `None` (`NoneType`)
  - ASCII strings (`'...'` and `"..."` with escapes `\n`, `\t`, `\r`, `\\`, `\'`, `\"`, `\0`)
  - Lists (`[elem, ...]`)
  - Dictionaries (`{key: val, ...}`)
- **Operators**:
  - **Arithmetic**: `+`, `-`, `*`, `/`, `%`, `**`
  - **Bitwise**: `&`, `|`, `^`, `~`, `<<`, `>>`
  - **Comparison**: `==`, `!=`, `<`, `<=`, `>`, `>=`
  - **Membership**: `in`, `not in`
  - **Logical**: `and`, `or`, `not` with short-circuit evaluation
- **Slicing & Indexing**:
  - 0-based and negative indexing (`obj[0]`, `obj[-1]`)
  - Slicing (`obj[start:stop]`, `obj[:stop]`, `obj[start:]`)
- **Control Expressions**:
  - Conditional expressions: `x if condition else y`
  - List comprehensions: `[expr for var in iterable]` and `[expr for var in iterable if cond]`
- **Built-in Functions**:
  - `len(x)`: Length of string, list, or dictionary
  - `print(...)`: Prints values separated by spaces followed by a newline
  - `str(x)`: Converts integer, bool, None to string
  - `int(x)`: Converts string or bool to integer
  - `range(stop)` / `range(start, stop)`: Generates a list of integers
  - `type(x)`: Returns the Python class name representation (e.g. `<class 'int'>`)
- **Program Exit**:
  - `exit(status)` (returns `status` via `$FF05`)
  - `exit()` (status 0)
  - `bye` (REPL exit)
  - EOF (`\x04` / Ctrl-D)
- **Far Memory & Far Functions**:
  - Memory for strings, lists, dictionaries, tokens, and symbol tables is allocated on the **Far Heap** (Blocks 128..250) using `FarMakeSlice`, `FarAlloc`, and `FarFree`.
  - All functions are partitioned into 8KB physical Far blocks (Blocks 8..127) with trampolines in Slot 6.
  - Struct `PyVal` uses a 12-byte layout for machine word alignment on the 6809.
- **Panic Recovery & Destructors**:
  - `defer func() { recover() }` catches runtime errors (e.g., `ZeroDivisionError`, `NameError`, `IndexError`, `SyntaxError`) and prints the error message without crashing the REPL.
  - `TokenGuard` uses a MiniGolf `destructor()` method to automatically release allocated token buffers on normal exit or panic unwinding.

## Files

- [`pyeval.golf`](pyeval.golf): Evaluator, parser, tokenizer, runtime, and REPL source code in MiniGolf.
- [`Makefile`](Makefile): Compiles `pyeval.golf` into `build/pyeval.decb`, generating all build artifacts (`build/pyeval.asm`, `build/pyeval.list`, `build/mode81.tcl`).
- [`run_py_gep9.sh`](run_py_gep9.sh): Launches `build/pyeval.decb` in the `gep9` emulator with an interactive REPL console.
- [`run_py_tfr911.sh`](run_py_tfr911.sh): Launches `build/pyeval.decb` on the physical TFR911H hardware board using `ENGINE_GEP9BIG` with an interactive REPL console.
- [`test_pyeval.sh`](test_pyeval.sh): Test runner that compiles `pyeval.golf`, runs on `gep9`, and checks output against `.want`.
- [`test_input.txt`](test_input.txt): Comprehensive test script covering all expressions, operations, builtins, and error recoveries.
- [`test_output.want`](test_output.want): Expected test output.

## Building and Running

### Build with Make
```bash
make -C python
# or inside python/:
make
```
Artifacts are created in `python/build/`:
- `build/pyeval.asm`: Generated 6809 assembly from MiniGolf compiler.
- `build/pyeval.list`: Disassembly listing from `asm6809`.
- `build/pyeval.decb`: Assembled Extended DECB binary with Far bank trampolines.
- `build/mode81.tcl`: Boot configuration for TFR911H `ENGINE_GEP9BIG`.

### Run Interactive REPL on `gep9` Emulator
```bash
./python/run_py_gep9.sh
```

### Run Interactive REPL on TFR911H Hardware Board
```bash
./python/run_py_tfr911.sh
```

### Run Automated Test Suite
```bash
make -C python test
# or:
./python/test_pyeval.sh
```
