# Source-Level Feature Comparison: Par3, Par4, Par5, MiniGolf, and Go

This document compares what is allowed at the **source-code level** across the five languages in the MiniGolf family, ordered from the most restricted subset (**Par3**) to full standard **Go**:

$$\textbf{Par3} \subset \textbf{Par4} \subset \textbf{Par5} \subset \textbf{MiniGolf} \subset \textbf{Go}\text{ (syntax)}$$

---

## 1. Quick Overview of the Five Languages

| Language | File Ext / Flag | Primary Role & Target Environment |
| :--- | :--- | :--- |
| **Par3** | `*.par3` / `*.go`<br/>`minigolf_par3` | Frozen self-hosting P-code subset designed so the 2-pass compiler ([`par3-apps/par3comp.par3.and.go`](../par3-apps/par3comp.par3.and.go)) is valid standard Go (bootstrapped directly via `go build par3-apps/par3comp.par3.and.go par3-apps/par3comp_gosupport.go`) and can compile itself inside a single 64 KB process under NitrOS-9 on M6809. |
| **Par4** | `*.par4`<br/>`minigolf -m=par4` | Evolution of Par3 P-code (originally called `NP`) targeting the `par4vm*` bytecode engines (`par4vmp` on POSIX, `par4vm9` on 6809, `par4vmc` on RCA 1802) for Hatvan OS. Adds user structs, methods, local buffers, multi-package imports, and Hatvan kernel traps, while keeping Par3's unsigned-only (`word`/`byte`) and non-generic rules. |
| **Par5** | `*.golf`<br/>`minigolf -m=big6809`<br/>`minigolf -m=6809 -membiggen` | **Embiggened** MiniGolf targeting the 2 MB paged virtual memory model (8 KB MMU windows). Supports almost all of MiniGolf (including `int`, generics, `defer`, `panic`/`recover`), but uses **far function pointers/trampolines** and **far data** for the backing contents of `string`, `slice[T]`, and `Smap[T]`/`Imap[T]` (8-byte slice headers `{FarRef, Base, Len, Cap}`). |
| **MiniGolf** | `*.golf`<br/>`-m=6809`, `-m=6502`, `-m=z80`, `-m=1802`, `-m=m68k`, `-m=amd64`, `-m=cbe` | Full flat-memory MiniGolf language. Every valid MiniGolf file is valid Go syntax (`gofmt`-compatible), but with C99-like value/memory semantics, manual memory management, optional RAII struct destructors, and 6-byte (3-word) unified slices/strings `{Base, Cap, Len}`. |
| **Go** | `*.go`<br/>`go build` | Standard Go (1.22+). Garbage-collected, with built-in concurrency (`go`, `chan`), interfaces, closures, built-in hash maps, and distinct immutable UTF-8 `string` vs `[]T`. |

---

## 2. Master Feature Support Matrix

**Legend:**
* **Yes** — Fully supported in source code.
* **No** — Forbidden / rejected at compile time (or not part of the language).
* **Host only** — Supported when compiled with the Go host compiler (`minigolf -m=par3`), but **not** by the self-hosted 6809 compiler (`par3comp.par3.and.go`).
* **Special / Lib** — Supported with distinct semantics or via a standard library package rather than a language built-in.

### 2.1 Primitive & Scalar Types

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| `byte` / `uint8` (8-bit unsigned) | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| `word` / `uint` / `uint16` (unsigned integer) | **Yes** (16-bit) | **Yes** (16-bit) | **Yes** (16-bit) | **Yes** (16-bit; 32/64-bit on 68k/amd64) | `uint` / `uint16` (`word` not builtin) |
| **Signed `int` / `int16`** | **No** *(panics; use `0xFFFF` sentinel)* | **No** *(panics; use `0xFFFF` sentinel)* | **Yes** (16-bit signed) | **Yes** (16-bit signed; 32/64-bit on 68k/amd64) | **Yes** (`int`, `int16`) |
| `int8`, `int32`, `int64`, `uint32`, `uint64` | **No** | **No** | **Lib** (`long`, `longlong`) | **Lib** (`long`, `longlong`) | **Yes** (builtin) |
| `float32`, `float64`, `complex64`, `complex128` | **No** | **No** | **Lib** (`floating`, `complex`) | **Lib** (`floating`, `complex`) | **Yes** (builtin) |
| `bool` (`true`, `false`) | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| Implicit integer $\leftrightarrow$ `bool` in `if` / `for` (`if x { ... }`) | **Yes** | **Yes** | **Yes** | **Yes** | **No** *(must write `if x != 0`)* |
| `noreturn` return pseudo-type | **Yes** | **Yes** | **Yes** | **Yes** | **No** |
| Untyped integer constants (`const N = 100`) | **Yes** *(Self: literal only; Host: expr)* | **Yes** (const exprs) | **Yes** (const exprs) | **Yes** (const exprs) | **Yes** |
| `$FFFF` hex / `%1010` binary / `@755` octal literals | `$FFFF` & `0xFFFF` | **Yes** (all) | **Yes** (all) | **Yes** (all) | `0x`, `0b`, `0o` only (no `$`, `%`, `@`) |

---

### 2.2 Strings, Slices, and Maps

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| Slice type syntax | `slice[T]` or `[]T` *(Self: `slice` / `string`)* | `slice[T]` or `[]T` | `slice[T]` | `slice[T]` | `[]T` |
| **Slice / `string` header layout & size** | **6 bytes** (`{Base, Cap, Len}`) | **6 bytes** (`{Base, Cap, Len}`) | **8 bytes** (`{FarRef, Base, Len, Cap}`) | **6 bytes** (3 words: `{Base, Cap, Len}`) | `string`: 2 words (`{ptr, len}`)<br/>`[]T`: 3 words (`{ptr, len, cap}`) |
| **Where `string` / `slice` contents live** | Near 64 KB RAM | Near 64 KB RAM | **Far Data** (`FarRef != 0`) or Near literals (`FarRef == 0`) | Near flat RAM | GC Heap / Read-only data |
| `string` is an alias for `slice[byte]` | **Yes** | **Yes** | **Yes** | **Yes** | **No** (distinct types) |
| Mutable string bytes (`s[i] = 'A'`) | **Yes** (if in RAM) | **Yes** (if in RAM) | **Yes** (via `s[i]` / `s.Put(i, 'A')`) | **Yes** (if in RAM) | **No** (`string` is immutable) |
| Direct header field access (`s.Base`, `s.Cap`, `s.Len`) | **Yes** | **Yes** | **Yes** (+ `s.FarRef`; `s.Base` is block offset!) | **Yes** | **No** (use `len(s)`, `cap(s)`, `unsafe.SliceData`) |
| Cast `string` / `slice[byte]` directly to `*byte` | `(*byte)(s.Base)` | `(*byte)(s.Base)` | **Near literals only** *(Far slices have `FarRef != 0`, not a flat `*byte`!)* | **Yes** (`(*byte)(s)` or `(*byte)(s.Base)`) | **No** |
| Sub-slicing `s[lo:hi]`, `s[:hi]`, `s[lo:]` | **Yes** *(Self: requires both `s[lo:hi]`)* | **Yes** | **Yes** ($O(1)$ offset adjust) | **Yes** | **Yes** |
| Slice `.Chop(lo, hi)`, `.Get(i)`, `.Put(i, v)`, `.Pop()` | **Host only** | **Yes** | **Yes** (+ `.Get1`, `.Put1` for 2nd window) | **Yes** | **No** (use `s[lo:hi]`, `s[i]`) |
| Growing a slice | `s.Append(x)` or `append(s, x)` *(Host)* | `s.Append(x)` or `append(s, x)` | `s.Append(x)` (mutates in place) | `s.Append(x)` (mutates in place) | `s = append(s, x)` (returns new slice) |
| Allocating a slice | `makeslice[T](cap)` / `make` | `makeslice[T](cap)` / `make` | `makeslice[T](n)` / `FarMakeSlice[T](n)` | `makeslice[T](len, cap)` | `make([]T, len, cap)` |
| Freeing a slice (`freeslice(s)` / `strfree(s)`) | `free(s.Base)` | `free(s.Base)` | `freeslice(s)` / `strfree(s)` (`FarFree`) | `freeslice(s)` / `strfree(s)` | **No** (Garbage Collected) |
| String concatenation `s1 + s2` and comparisons `==`, `<` | **Yes** *(Self: comparisons only, no `+`)* | **Yes** | **Yes** | **Yes** | **Yes** |
| **Maps / Dictionaries** | VM intrinsics (`map_new`/`map_get`/`map_put` or `Smap[T]`) | VM intrinsics (`Smap[T]` / `DICT_*` / `MAP_*`) | Generic `smap.Smap[T]` & `smap.Imap[T]` in Far Data | Generic `smap.Smap[T]` & `smap.Imap[T]` in `golflib` | Built-in `map[K]V` |
| Interned String Map (`Imap[T]`) fast equality | N/A | N/A | Compares `FarRef` **and** `Base` (32-bit) | Compares `Base` (16-bit) | N/A |

---

### 2.3 Structs, Arrays, Pointers, and Memory

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **User-defined `type Name struct { ... }`** | **Host only** *(Not in self-hosted `minigolf_par3`)* | **Yes** (concrete fields only) | **Yes** (concrete & generic) | **Yes** (concrete & generic) | **Yes** |
| Struct composite literals (`Point{X: 1, Y: 2}`) | **Host only** | **Yes** | **Yes** | **Yes** | **Yes** |
| Passing/assigning structs by value (`a = b`) | **Host only** *(prefer `*T`)* | **Yes** (via `MEM_COPY`; prefer `*T` if large) | **Yes** | **Yes** (unless struct has `destructor()`) | **Yes** |
| Fixed-size global arrays (`var a [N]T`) | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| **Fixed-size local stack arrays (`var buf [N]byte`)** | **No** *(Forbidden in `-m=par3`; use global or `alloc()`)* | **Yes** *(if total local frame $\le 255$ bytes)* | **Yes** (in Slot 1 stack) | **Yes** | **Yes** |
| Array composite literals (`[3]word{1, 2, 3}`) | **No** | **Yes** | **Yes** | **Yes** | **Yes** |
| Pointers (`*T`), address-of (`&x`), dereference (`*p`) | **Yes** *(Self: `*p` is 16-bit `PEEK2`)* | **Yes** | **Yes** (Near pointers in fixed RAM; Far data uses `FarRef`) | **Yes** | **Yes** |
| **Pointer arithmetic / stepping (`p++`, `p--`, `p += n`, `p -= n`)** | **Yes** (unscaled word in Self; scaled in Host) | **Yes** | **Near pointers only** *(Forbidden on `FarRef`/slices)* | **Yes** (scaled by `sizeof[T]()`) | **No** |
| Raw address cast (`(*T)(addr)`, `word(ptr)`) | **Yes** | **Yes** | **Near addresses only** | **Yes** | **No** (requires `unsafe.Pointer`) |
| `peek[T](addr)`, `poke[T](addr, val)`, `peekb`, `pokeb`, `peekw`, `pokew` | **Yes** | **Yes** (+ `vpeek*`/`vpoke*`) | **Yes** (for Near / mapped window addresses) | **Yes** (+ `vpeek*`/`vpoke*`) | **No** |
| `sizeof[T]()` compile-time size | **Host only** (`sizeof[T]()`) | **Yes** (`sizeof[T]()`) | **Yes** (`sizeof[T]()`) | **Yes** (`sizeof[T]()`) | `unsafe.Sizeof(x)` |
| Dynamic heap allocation | `alloc(n)` / `free(addr)` | `alloc(n)` / `free(addr)` | Near: `malloc(n)`/`free(p)`<br/>Far: `FarAlloc(n)`/`FarFree(blk, off)` | `malloc(n)` / `zalloc(n)` / `free(p)` | Automatic (`new`, `make`, GC) |

---

### 2.4 Generics & Type System

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **User-defined generic structs (`type Box[T any] struct`)** | **No** | **No** | **Yes** (monomorphized) | **Yes** (monomorphized) | **Yes** |
| **User-defined generic functions/methods (`func Foo[T any](...)`)** | **No** | **No** | **Yes** (monomorphized) | **Yes** (monomorphized) | **Yes** |
| Built-in generic syntax (`slice[T]`, `peek[T]`, `poke[T]`, `sizeof[T]`) | **Host only** (`Smap[T]`, `slice[T]`) | **Yes** (compiler intrinsics only) | **Yes** | **Yes** | `[]T` (no `peek`/`poke`/`sizeof[T]`) |
| `any` type (dynamic boxing `{BaseAddr, TypeStr}`) | Only inside `print`/`println` | Only inside `print`/`println` | **Yes** | **Yes** | **Yes** (`any` / `interface{}`) |
| Interfaces with method sets (`type Reader interface { ... }`) | **No** | **No** | **No** | **No** | **Yes** |
| Type assertions (`x.(T)`) & type switches (`switch x.(type)`) | **No** | **No** | **No** | **No** | **Yes** |
| Type aliases / definitions (`type Word2 word`) | **No** | **No** | **No** (structs only) | **No** (structs only) | **Yes** |

---

### 2.5 Functions & Methods

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| Multiple return values (`func f() (T1, T2)`) | **Yes** (up to 2 in Self) | **Yes** | **Yes** | **Yes** | **Yes** |
| **Named return values (`func f() (n word)`) & bare `return`** | **No** *(panics)* | **No** *(panics)* | **Yes** | **Yes** | **Yes** |
| Blank identifier `_` in assignments (`_, ok := f()`) | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| Pointer-receiver methods (`func (p *T) m()`) | **Host only** *(Not in Self)* | **Yes** | **Yes** | **Yes** | **Yes** |
| Value-receiver methods (`func (v T) m()`) | **No** | **No** | **No** *(use pointer receiver `*T`)* | **No** *(use pointer receiver `*T`)* | **Yes** |
| Auto-referencing `s.m()` when `m` takes `(p *T)` | **Host only** | **Yes** | **Yes** | **Yes** | **Yes** |
| **First-class function pointers (`var fn func(word) word`)** | **No** | **No** | **Yes** (via Slot 6 trampolines) | **Yes** | **Yes** |
| Anonymous functions / closures capturing outer locals | **No** | **No** | Only `defer func() { ... }()` | Only `defer func() { ... }()` | **Yes** (full closures) |
| User-defined variadic functions (`func f(args ...T)`) | **No** *(only `print`/`println`)* | **No** *(only `print`/`println`)* | **Yes** | **Yes** | **Yes** |
| Package `func init()` | **Host only** | **Yes** | **Yes** | **Yes** | **Yes** |

---

### 2.6 Statements, Expressions, and Control Flow

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| `if cond { ... } else if ... else { ... }` | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| `if init; cond { ... }` (init statement in `if`) | **No** | **No** | **No** | **No** | **Yes** |
| `for { ... }` and `for cond { ... }` | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| `for init; cond; post { ... }` | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| `for i := range N` (integer range $0 \dots N-1$) | **Host only** *(Not in Self)* | **Yes** | **Yes** | **Yes** | **Yes** (Go 1.22+) |
| `for i, v := range sliceOrArray` | **Host only** *(Not in Self)* | **Yes** | **Yes** | **Yes** | **Yes** |
| **`switch x { case ...: default: }` and `switch { ... }`** | **Host only** *(Not in Self)* | **Yes** (int, bool, string) | **Yes** (int, bool, string) | **Yes** (int, bool, string) | **Yes** |
| `break` and `continue` (unlabeled) | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| Labeled `break label` and `continue label` | **No** | **No** | **Yes** | **Yes** | **Yes** |
| `goto label` and `label:` | **Host only** *(Not in Self)* | **Yes** | **Yes** | **Yes** | **Yes** |
| Ternary expression `cond(pred, yesExpr, noExpr)` | **Host only** | **Yes** | **Yes** | **Yes** | **No** |
| Bit-clear operator `&^` and `&^=` | **No** | **Yes** | **Yes** | **Yes** | **Yes** |
| Bitwise complement unary prefix `^x` or `~x` | **Yes** (both `^x` and `~x`) | **Yes** (`^x`) | **Yes** (`^x`) | **Yes** (`^x`) | `^x` only |
| Compound assignments (`+=`, `-=`, `*=`, `/=`, `%=`, `&=`, `|=`, `^=`, `<<=`, `>>=`) | **Yes** *(Self: `+=` and `-=` only)* | **Yes** | **Yes** | **Yes** | **Yes** |
| Tuple / parallel assignment (`a, b = b, a`) | **Host only** *(Self: `a, b := fn()` only)* | **Yes** | **Yes** | **Yes** | **Yes** |

---

### 2.7 Error Handling, `defer`, Destructors, and Runtime

| Feature / Construct | Par3 | Par4 | Par5 (Embiggen) | MiniGolf (Flat) | Go |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **`defer` statement (`defer f()`, `defer func(){...}()`)** | **No** *(panics)* | **No** *(panics)* | **Yes** *(at top level of func body)* | **Yes** *(at top level of func body)* | **Yes** *(anywhere in func)* |
| **`panic(msg)`** | **Yes** (halts VM immediately) | **Yes** (halts VM immediately) | **Yes** (unwinds `defer` & destructors) | **Yes** (unwinds `defer` & destructors) | **Yes** (unwinds `defer`s) |
| **`recover()`** | **No** | **No** | **Yes** (`recover() *byte`) | **Yes** (`recover() *byte`) | **Yes** (`recover() any`) |
| **Automatic Struct Destructors (`func (p *T) destructor()`)** | **No** | **No** | **Yes** (RAII on scope exit & panic) | **Yes** (RAII on scope exit & panic) | **No** |
| **Packages & `import "pkg"`** | **Host only** *(Self is single-file `package main`)* | **Yes** | **Yes** | **Yes** | **Yes** |
| Identifier visibility across packages | All exported | All exported | All exported (upper & lower case) | All exported (upper & lower case) | Capitalized identifiers only |
| Goroutines (`go f()`), Channels (`chan T`), `select` | **No** | **No** | **No** | **No** | **Yes** |

---

## 3. Summary of Key Pitfalls & "Gotchas" When Moving Between Dialects

1. **Par3 vs. Par4**:
   * **Both forbid signed `int`**: Any variable, parameter, or cast using `int` or `int16` will panic at compile time. Always use `word` or `byte`, and use `0xFFFF` instead of `-1` for error/not-found sentinels.
   * **Both forbid user-defined generics, `defer`, and named return values**: Functions with return types must end with an explicit `return <expr>`.
   * **Stack buffers (`var buf [N]byte`)**: Forbidden in Par3 (`-m=par3`), but **allowed in Par4** as long as the function's total local variables fit within 255 bytes.
   * **Self-hosted Par3 (`par3-apps/par3comp.par3.and.go`)**: If a `.par3` file must be compiled on the 6809 by `minigolf_par3` itself, stick to a single file (`package main`), no user `struct` declarations, no `switch`, no `for ... range`, and only `+=` / `-=` op-assignments.

2. **Par4 vs. Par5 (Embiggen)**:
   * **Par5 unlocks the full MiniGolf language**: Signed `int`, user-defined generics (`[T any]`), `defer`, `panic`/`recover`, struct `destructor()`, named return values, and function pointers all work in Par5.
   * **Slices and Strings are 8 bytes in Par5 (`{FarRef, Base, Len, Cap}`) instead of 6 bytes (`{Base, Cap, Len}`)**:
     * In Par5, dynamically allocated strings and slices store their payload in **Far Data** (`FarRef != 0`), where `s.Base` is an offset inside an 8 KB block, **not** a flat 16-bit CPU pointer.
     * Never cast a Par5 slice's `.Base` to `*byte` to read/write elements directly; always use `s[i]`, `s.Get(i)` / `s.Put(i, x)` (or `s.Get1(i)` / `s.Put1(i, x)` when copying between two far slices simultaneously).
     * When comparing interned strings by address in Par5 (e.g., in `Imap`), compare both words: `a.FarRef == b.FarRef && a.Base == b.Base`.

3. **MiniGolf ( & Par5) vs. Go**:
   * **Slices mutate in place on `.Append(x)`**: Write `s.Append(x)` (a method call on `*slice[T]`), not `s = append(s, x)`.
   * **`defer` placement**: In MiniGolf and Par5, `defer` statements must be at the top statement level of a function body (not nested inside `if` or `for` blocks).
   * **Methods require pointer receivers**: Always declare methods as `func (p *MyStruct) Foo()`, never with a value receiver `func (v MyStruct) Foo()`.
   * **No Garbage Collector**: Use `malloc`/`free`, `freeslice`/`strfree`, `defer`, or `func (p *T) destructor()` to avoid exhausting heap memory on 8-bit targets.

---

## Appendix: Detailed Differences Between Par3 and Par4

### A.1 Mission, Naming History (`NP` $\rightarrow$ `Par3`), and Compilation Pipelines

* **Par3 (`*.par3` / `par3comp.par3.and.go`, formerly called `NP` / `MiniGolf-NP`)**:
  * **Mission**: Frozen and preserved specifically to **self-host** the NitrOS-9 build toolchain (`minigolf_par3`, `par3asm`, `par3pack`, `selfgen_preprocess`, `build_img`) inside a single 64 KB Motorola 6809 process ([`par3-apps/selfgen-on-gep9.sh`](../par3-apps/selfgen-on-gep9.sh)).
  * **Direct Go Bootstrap (No `np/` Needed)**: [`par3-apps/par3comp.par3.and.go`](../par3-apps/par3comp.par3.and.go) is valid standard Go when compiled alongside [`par3-apps/par3comp_gosupport.go`](../par3-apps/par3comp_gosupport.go) (`go build par3-apps/par3comp.par3.and.go par3-apps/par3comp_gosupport.go`). When `minigolf_par3` compiles itself (or other `.par3` tools), `par3comp_gosupport.go` is omitted and those runtime helpers (`mk_str`, `str_base`, `free_str`, `get_arg`, `file_*`, `map_*`, `sys_*`) are emitted directly as VM bytecode primitives executed by [`par3-runtime/par3runtime.asm`](../par3-runtime/par3runtime.asm). Because `par3comp.par3.and.go` bootstraps directly with Go, the legacy `np/` host backend has been retired.
  * **4-Step NitrOS-9 Build Pipeline**:
    1. `minigolf_par3 foo.par3 foo.p3a` — compiles `.par3` / `.go` source into `.p3a` stack-VM assembly text.
    2. `par3asm foo.p3a -o foo.p3p` — assembles `.p3a` into `.p3p` binary P-code bytecodes.
    3. `par3pack foo.p3p foo` — converts `.p3p` bytes into `payload.asm` (`fcb $xx,...` directives) and `modname.asm`.
    4. `asm par3runtime.prep.asm -o=foo` — assembles the 6809 P-code interpreter ([`par3-runtime/par3runtime.asm`](../par3-runtime/par3runtime.asm)) together with `payload.asm` into a standalone NitrOS-9 executable module.

* **Par4 (`*.par4` / `*.golf` with `minigolf -m=par4`)**:
  * **Mission**: Because Par3 is frozen to protect the NitrOS-9 self-hosting bootstrap, [`par4/`](../par4/codegen.go) was forked and evolved from `np/` as the primary P-code bytecode target for **Hatvan OS** across three VM engines:
    * **`par4vmp`** ([`par4/par4vmp.go`](../par4/par4vmp.go)): Fast POSIX/PC reference engine and debugger written in Go.
    * **`par4vm9`**: Motorola 6809 / Hitachi 6309 assembly engine for `gep9` and TFR911h hardware.
    * **`par4vmc`**: RCA COSMAC CDP1802 assembly engine (~3 KB interpreter for 1802 systems where native code is too large).
  * **Pure Go Host Toolchain**: Compiled by the full MiniGolf frontend + [`par4/codegen.go`](../par4/codegen.go), assembled by `par4asm` ([`par4/par4asm.go`](../par4/par4asm.go)), and disassembled by `par4dis` ([`par4/par4dis.go`](../par4/par4dis.go)) with zero Python dependencies.

---

### A.2 Source-Level Differences: Par3 (Self-Hosted) vs. Par3 (Host `-m=par3`) vs. Par4

Both Par3 and Par4 enforce the core P-code simplifications (**no signed `int`**, **no user-defined generics**, **no `defer`**, **no `recover()`**, **no named return values**, **no function pointers**) and use **6-byte near slices** (`{Base, Cap, Len}`).

However, Par4 unlocks many MiniGolf source constructs that are restricted in Par3:

| Source Construct | Par3 (Self-Hosted `minigolf_par3`) | Par3 (Host `-m=par3`) | Par4 (`minigolf -m=par4`) |
| :--- | :--- | :--- | :--- |
| **Multi-package `import`** | **No** (single-file `package main` only) | **Yes** | **Yes** (with cross-package `pkg.CONST` pre-pass) |
| **User `type T struct { ... }`** | **No** (only `.Base`, `.Cap`, `.Len` on slices) | **Yes** | **Yes** (including nested structs & `MEM_COPY` assignment) |
| **Struct methods `func (p *T) m()`** | **No** | **Yes** | **Yes** |
| **Local stack arrays `var buf [N]byte`** | **Yes** (small arrays) | **No** *(panics: "buffers on the stack are not supported")* | **Yes** (up to 255-byte local frame) |
| **Global `var` initializers (`var x = 5`)** | **No** (uninitialized `var x word` only) | **Yes** | **Yes** |
| **`const` expressions (`const B = A + 1`)** | **No** (integer literals only, max 48) | **Yes** | **Yes** |
| **`for i := range N` & `for i, v := range s`** | **No** (only `for cond` and 3-clause `for`) | **Yes** | **Yes** |
| **`switch` / `case` / `default`** | **No** (`if` / `else if` only) | **Yes** | **Yes** (int, bool, string, and `SWITCH_LOOKUP` tables) |
| **`goto` and labels** | **No** | **Yes** | **Yes** |
| **Compound assignments** | `+=` and `-=` only | All except `&^=` | All including `&^` and `&^=` |
| **Tuple assignment (`a, b = b, a`)** | Only `a, b := fn()` for 2-return calls | **Yes** | **Yes** |
| **OS / Hardware Builtins** | `file_open_read`, `file_open_write`, `file_readline`, `file_write`, `file_close`, `sys_args`, `sys_exit` | Same as self-hosted | Adds `hatvan_trap(a,b,x,y,u,call)`, `sys_poll_flag(n)`, `sys_dma_copy(...)`, `memcpy`, `memset`, `vpeek*`/`vpoke*` |

---

### A.3 Bytecode & VM Instruction Set Differences (`.p3p` vs. `.p4p`)

Comparing [`par3-runtime/opcodes.py`](../par3-runtime/opcodes.py) with [`par4/opcodes.go`](../par4/opcodes.go):

1. **System Call Architecture (`0xC0..0xCF`)**:
   * **Par3** hardcodes high-level NitrOS-9 / POSIX file and OS opcodes in Group 8: `FILE_OPEN_READ` (`0xC0`), `FILE_OPEN_WRITE` (`0xC1`), `FILE_READLINE` (`0xC2`), `FILE_WRITE` (`0xC3`), `FILE_CLOSE` (`0xC4`), `OS_ISFILE` (`0xC5`), `OS_MAKEDIRS` (`0xC6`), `SYS_ARGS` (`0xC7`), `FILE_READ` (`0xCC`), `FILE_WRITE_BUF` (`0xCD`).
   * **Par4** replaces those file opcodes with low-level **Hatvan Kernel Trap & Hardware Primitives**:
     * `HATVAN_TRAP` (`0xC0`): Unified OS-9 / Hatvan register-based kernel syscall (`A, B, X, Y, U, TrapNum` $\rightarrow$ `A, B, X, Y`).
     * `SYS_POLL_FLAG` (`0xC1`): Hardware event flag polling (e.g. 1802 `EF1..EF4`).
     * `SYS_DMA_COPY` (`0xC2`): Cross-task curtain / DMA memory transfer.
2. **New Stack, Memory, and Control Opcodes in Par4**:
   * **Stack**: Par4 adds `DUP2` (`0x10`) and `ROT` (`0x11`).
   * **Memory**: Par4 replaces `ZALLOC` (`0x37`) with `MEM_COPY` (`0x37`) and `MEM_SET` (`0x38`) for fast struct/array copying and zeroing.
   * **Control Flow**: Par4 adds `PANIC` (`0x68`) and `SWITCH_LOOKUP` (`0x69`, compact jump-table switch dispatch).
3. **Binary Header, Global Table, and Synthetic Locals**:
   * **Par3 (`.p3p`)**: Magic `"P3P\x01"`. Always emits synthetic scratch locals (`_pbuf`, `_tmp_base`, `_tmp_cap`, `_tmp_len`, `_tmp_addr`) in every function frame.
   * **Par4 (`.p4p`)**: Magic `"P4P\x01"`. Uses 2-byte (`uint16`) sizes in the Global Variable Table so global structs and arrays up to 64 KB work across `par4vmp`, `par4vm9`, and `par4vmc`, supports 1-byte (`byte`), 2-byte (`word`), 6-byte (`slice`), and $N$-byte (buffer/struct) local frame slots, and omits unused synthetic scratch locals (`_pbuf`, `_tmp_*`, `_swap_*`).

---

### A.4 Regular Expressions in Par3 and Par4

* **Whole-Word Identifier Replacement (`STR_REPLACE_IDENT`, `0x7F`)**: Neither Par3 nor Par4 has regex opcodes in the VM. For whole-word assembly identifier replacement in `selfgen_preprocess.par3`, Par3 and Par4 use the dedicated `STR_REPLACE_IDENT` (`0x7F`, `replace_ident(s, old, new)`) opcode.
* **Library Thompson NFA VM ([`par4-lib/regexp.par4`](../par4-lib/regexp.par4))**: General regular expressions (`c`, `.`, `|`, `*`, `?`, `(...)`) are implemented in pure Par4 via `import "regexp"`. Calling `re.Compile(pat)` compiles the pattern into a 5-opcode **Thompson NFA bytecode** (`INST_CHAR`, `INST_ANY`, `INST_JMP`, `INST_SPLIT`, `INST_MATCH`) packed as `(op << 8) | arg` words inside `re.Code [32]word`, which `re.MatchFull(s)` and `re.Match(s)` execute in strict $O(N)$ linear time.

---

### A.5 Dual-Valid Go / Par3 Design of `par3comp.par3.and.go`

By treating strings/slices as an opaque abstraction manipulated through function primitives (`mk_str(base, cap, len)`, `str_base(s)`, `free_str(s)`, `get_arg(args, idx)`, `len(s)`) rather than special `.Base` / `.Cap` / `.Len` field-selector syntax:
1. [`par3-apps/par3comp.par3.and.go`](../par3-apps/par3comp.par3.and.go) is **100% valid standard Go** when built with [`par3-apps/par3comp_gosupport.go`](../par3-apps/par3comp_gosupport.go).
2. The Par3 language and compiler no longer require `.` struct field access or field assignment syntax at all.
3. The VM runtime ([`par3-runtime/par3runtime.asm`](../par3-runtime/par3runtime.asm)) implements `FREE_STR` (`0xCE`) and `GET_ARG` (`0xCF`) alongside `SLICE_NEW` (`0x70`) and `SLICE_LEN` (`0x71`), allowing `par3comp.par3.and.go` to compile itself in 64 KB under NitrOS-9 Level 2 (`deep65280`).

