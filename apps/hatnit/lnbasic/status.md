# LNBASIC Status, Code-Size Postmortem, and Implementation Plan

## 1. Current Status

- **`apps/hatnit/lnbasic/strawman.md`**: Full language design strawman for the long-term LNBASIC (supporting `A`, `A$`, `A@`, `A%`, `A&`, `DEF FN`, ASCII + `.lnp` P-Code save/load, `runlnb`, and `.lnl` lazy library loading, plus an analysis of why Par4 P-Codes are not a fit for BASIC).
- **`apps/hatnit/lnbasic/lnbasic.golf`**: First complete implementation of a classic Radio Shack CoCo-style Line-Numbered BASIC interpreter (~2,357 lines of MiniGolf) using `package os9` for all I/O.
  - Compiles cleanly to 6809 assembly (`./minigolf -m 6809 -f os9` and `./minigolf -m 6809+`).
  - However, the resulting 6809 binary size was a major surprise: **58,887 bytes** (`34,723` lines of assembly) in standard `-m 6809 -f os9` mode, and **94,631 bytes** (`74` 8KB blocks) in `-m 6809+` mode.
  - Because `lnbasic` must run comfortably inside a **<64 KB** address space alongside its heap, variables, and tokenized program lines on both NitrOS-9 (Level 1 / Level 2) and HatvanOS, we paused here to analyze why the generated code was so large and how to change our strategy.

---

## 2. What We Learned: Why the Compilation Was So Large

### 2.1 Breakdown of Function Sizes in `_tmp/lnbasic_os9.asm`

| Function | Assembly Lines | Approx. Code Size | Root Cause |
| :--- | ---: | ---: | :--- |
| `f_main__tokenName` | 3,306 | ~5.6 KB | 52 `if kind == TOK_... { return "..." }` branches returning 6-byte `string` structs by value |
| `f_main__matchKeyword` | 2,718 | ~4.6 KB | 39 `if wordBuf == "..."` string comparisons constructing 6-byte `string` literals on the stack |
| `f_main__evalStrAtom` | 2,634 | ~4.5 KB | Passing/returning `string` (6B) and `Token` (11B) structs by value across ~25 call sites |
| `f_main__tokenizeStmts` | 2,493 | ~4.2 KB | Building 11-byte `Token` structs by value and calling `curTokens.Append(t)` |
| `f_main__execTokenAction` | 1,844 | ~3.1 KB | Calling `peekTok().Kind` dozens of times (each call copies an 11-byte `Token` struct by value) |
| `f_main__writeProgLineToPath` | 1,396 | ~2.4 KB | Calling `st.Tokens.Get(i)` (11-byte struct return) and passing many string literals (`" "`, `"\""`) |
| `slice[T]` instantiations | ~2,500 | ~4.2 KB | Separate generic monomorphizations for `slice[Token]`, `slice[int]`, `slice[string]`, `slice[byte]` |
| `f_main__evalIntRel` | 833 | ~1.4 KB | String and integer relational comparisons |
| `f_main__evalIntPrimary` | 708 | ~1.2 KB | Primary integer expressions and built-ins (`LEN`, `VAL`, `ASC`, `ABS`, `MEM`) |
| `f_main__execAssign` | 672 | ~1.1 KB | Scalar and `DIM` array assignments |

Just the top 6 functions account for **~25 KB** of 6809 machine code.

### 2.2 Three Fundamental Causes of the Code Explosion

#### Cause 1: Every `string` Literal or `struct` Value Return Costs ~110 Bytes on 6809
In MiniGolf's SSA IR (`ir/builder.go`), a `string` is a 6-byte aggregate `{Base word, Cap word, Len word}`. Every time a string literal like `"PRINT"` or `""` appears in an expression or `return` statement, the SSA builder emits a `ZeroInitializer` followed by three `InsertValue` instructions (one per struct field).

In `m6809/backend.go`, each `InsertValue` copies the entire 6-byte struct via `lbsr __memcpy`, writes 2 bytes, and copies 6 bytes back via `lbsr __memcpy`—and when `globalsAtY` (`-f os9`) is active, each `__memcpy` is followed by `tfr dp,a / clrb / tfr d,y` to restore `Y`:
```asm
	; What a single `return "PRINT"` compiles to in _tmp/lnbasic_os9.asm:
	leax 3,s
	ldd #6
	lbsr __memset0
	leax v_str_const_28,pcr
	tfr x,d
	std 9,s
	leax 11,s
	leay 3,s
	ldd #6
	lbsr __memcpy
	tfr dp,a
	clrb
	tfr d,y
	ldd 9,s
	std 11,s
	... (repeated 3 times for Base, Cap, Len!)
```
- **Cost**: A single `"..."` literal in source code compiles into **~42 instructions (~110 bytes)** and **24 bytes of local stack frame**.
- In `tokenName` (52 string literals) and `matchKeyword` (39 string literals), those 91 string literals alone generated **6,024 lines of assembly (~10.2 KB)** and allocated a **341-byte stack frame** in `tokenName`.

#### Cause 2: Returning and Passing `Token` (11 bytes) by Value Instead of a Flat Byte Stream
In `lnbasic.golf`, we defined:
```go
type Token struct {
	Kind   byte   // 1B
	VarID  word   // 2B
	IntVal int    // 2B
	StrVal string // 6B
} // 11 bytes total
```
And wrote `peekTok() Token` and `nextTok() Token` returning `Token` by value.
- Every time the parser or evaluator checked `if peekTok().Kind == TOK_COMMA`, the 6809 had to reserve an 11-byte return buffer on the stack, call `peekTok()`, call `slice_Token_Get` (copying 11 bytes via `__memcpy`), copy 11 bytes again to the caller's frame via `__memcpy`, and then read just 1 byte (`Kind`) at offset 0.
- By contrast, in classic Microsoft / CoCo Color BASIC, a tokenized line is a **flat `*byte` stream** where peeking the next token is a single `peekb(pc)` (2 instructions instead of ~60).

#### Cause 3: `big6809` (`-m 6809+`) Staging Limit and HatvanOS Process Model
- `compile-hatvan.sh` uses `-m 6809+` (`big6809`), which widens `string` and `slice` to 8 bytes (adding `FarRef`) and packs functions into 8KB MMU blocks staged sequentially in the DECB image starting at `$4000` before `cstart_embiggen` at `$C000` copies them into high RAM banks. When a program exceeds 4 blocks (32 KB), the staging area (`$4000..$BFFF`) overflows into `$C000` and overwrites `cstart_embiggen`.
- Furthermore, standard HatvanOS user commands (see `hatvan-os/build-cmd.sh`) are compiled with **standard `-m 6809`** (16-bit flat address space per user process), not `-m 6809+`. Keeping `lnbasic` compact (~12–18 KB of 6809 code) allows the exact same standard `-m 6809` code to run in a single 64K space on both NitrOS-9 and HatvanOS.

---

## 3. Recommended Strategy Changes

### 3.1 How to Refactor `lnbasic.golf` (Application-Level Strategy)

1. **Tokenize into a Flat `malloc`'d Byte Buffer (Classic CoCo BASIC Style)**:
   - Replace `type Token struct` (11B) and `slice[Token]` with a compact byte stream per statement/line (`*byte` + `word` length, or 0-terminated token stream):
     - `0x00` (`TOK_EOF`): End of statement/line.
     - `0x01..0x4F`: Single-byte keyword, built-in function, or operator token (`PRINT`, `LET`, `IF`, `THEN`, `+`, `<=`, etc.).
     - `TOK_INT` (1B) + `hi, lo` (2B): Inline 16-bit integer literal (3 bytes total instead of 11).
     - `TOK_VAR_INT` / `TOK_VAR_STR` (1B) + `c0, c1` (2B): Inline variable ID (3 bytes total instead of 11).
     - `TOK_STR` / `TOK_REM_TEXT` (1B) + `ptr_hi, ptr_lo` (2B): Inline `word` pointer to a `malloc`'d heap string!
   - **Impact**:
     - `peekTok()` becomes `peekb(ExprPtr)` (a 2-byte scalar load in `D`/`B` instead of an 11-byte struct return via `__memcpy`).
     - Eliminates `slice[Token]` generic instantiations (`makeslice_Token`, `slice_Token_Get`, `slice_Token_Put`, `slice_Token_Append`, `freeslice_Token`).
     - Cuts RAM usage of stored BASIC programs by **3x–4x**.

2. **Use a Single Packed Keyword Table Instead of 90 String Literals**:
   - Store all keywords and operators in one constant string:
     ```go
     const KeywordTable = "\x01PRINT\x02LET\x03REM\x04IF\x05THEN\x06ELSE\x07GOTO\x08GOSUB\x09RETURN..."
     ```
   - `matchKeyword(buf *byte, n word) byte` scans `KeywordTable` with `peekb` in ~25 lines of code.
   - `writeKeyword(path byte, tok byte)` finds `tok` in `KeywordTable` and writes the slice directly to `path` in ~20 lines of code.
   - **Impact**: Replaces ~6,024 lines of assembly (`tokenName` + `matchKeyword`, ~10.2 KB) with **~300 bytes** of code + a **~220-byte** table.

3. **Represent Owned Heap Strings as a 2-Byte `word` Pointer (`HStr`)**:
   - Instead of passing 6-byte `string` structs by value through every expression evaluator function (`evalStrExpr() string`, `evalStrAtom() string`) and storing 6-byte `string` structs in `DIM A$(n)`, represent an owned heap string as a `word` address (`HStr`) pointing to a `malloc(len + 3)` block:
     - `peekw(h)`: 16-bit length (`len`)
     - `h + 2 .. h + 2 + len - 1`: character bytes
     - `pokeb(h + 2 + len, 0)`: null terminator
   - **Impact**:
     - Passing and returning owned strings in `evalStrAtom` / `evalStrExpr` passes a single 2-byte `word` in register `D` instead of allocating 6-byte return slots and calling `__memcpy`.
     - `DIM A$(20)` allocates `21 * 2 = 42` bytes for the array spine instead of `21 * 6 = 126` bytes, and reuses `slice[word]` / raw `malloc` instead of instantiating `slice[string]`.
     - Still strictly obeys the rule: **every string is `malloc`'d, copied (never shared), and `free`'d immediately when overwritten or consumed**.

4. **Table-Driven Error Codes**:
   - Instead of 15 calls to `raiseError("SYNTAX ERROR")` (where each string literal call site costs ~110 bytes), pass a `byte` error code `raiseErr(ERR_SYNTAX)` and look up the message in a single packed error string.
   - **Impact**: Saves another **~1.5 KB** of code.

### 3.2 Compiler-Level Opportunity (`m6809` / `ir`)

- **Optimize `InsertValue` / String Literal Construction in `m6809` (`ir/builder.go` & `m6809/backend.go`)**:
  - Currently, constructing a 3-word `string` or `slice` value in SSA emits `ZeroInitializer` + 3 `InsertValue` instructions, and `m6809` lowers each `InsertValue` as a full struct `__memcpy` to a new stack slot.
  - Modifying `InsertValue` in-place (or emitting a direct 3-word store for `StringLiteral` in `m6809`) will reduce every string literal from ~42 instructions (~110 bytes) down to **6 instructions (~16 bytes)** across all MiniGolf programs.

---

## 4. Original Implementation Plan (CoCo-Style Line-Numbered BASIC)

For reference when we resume work on `apps/hatnit/lnbasic/lnbasic.golf`, here is the original specification and plan:

### Goal
Build a self-contained, interactive CoCo-style Line-Numbered BASIC interpreter in [`apps/hatnit/lnbasic/lnbasic.golf`](file:///home/strick/github.com/strickyak/minigolf/apps/hatnit/lnbasic/lnbasic.golf) targeting NitrOS-9 and HatvanOS via `package os9`:
- **Interactive stdin/stdout** (`OK` prompt) + immediate mode + numbered program lines (`1..65529`, enter empty line number to delete).
- **Commands**: `RUN`, `LIST`, `SAVE "file"`, `LOAD "file"`, `NEW`, `CLEAR`, `BYE` / `SYSTEM`.
- **Statements**: `PRINT`, `LET` (optional keyword: `LET A = 1` or `A = 1`), `REM` (and `'`), `IF ... THEN ... [ELSE ...]`, `GOTO`, `GOSUB`, `RETURN`, `DIM`, `FOR ... TO ... [STEP ...] / NEXT`, `INPUT`, `END` / `STOP`.
- **Types & Arrays**:
  - `A`..`Z` (and 2-char `A0`..`Z9`, `AA`..`ZZ`) `int` (`int16_t`) scalars and `DIM A(n)` arrays.
  - `A$`..`Z$` (and 2-char `A0$`..`ZZ$`) `string` scalars and `DIM A$(n)` string arrays.
- **Strict `malloc` & `free` Discipline (No Shared Strings)**:
  - Every string value (in `A$`, `A$(i)`, string literals in tokens, `REM` text, and intermediate string expressions like `A$ + B$` or `MID$`) owns its own `malloc`ed buffer and is `free`d as soon as it is overwritten or consumed.
  - Every `DIM` array (`A(n)`, `A$(n)`), every `ProgLine`, every `Stmt`, and every token buffer is `malloc`ed and `free`d on line replacement, line deletion, `CLEAR`, or `NEW`.
  - Expose `MEM` built-in function (`PRINT MEM` -> `BytesFree()`) to verify zero memory leaks across `NEW`, line replacements, and string loops.
- **Tokenized Statements**:
  - Lines are tokenized on entry into `malloc`ed token buffers for fast execution and clean detokenization on `LIST` / `SAVE`.
