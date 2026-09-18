# Known Bugs and Limitations in the MiniGolf C Compiler Pipeline

This document catalogs all known bugs, translation quirks, and architectural limitations in the MiniGolf C compilation pipeline:
- **`ctranslator`** (C99 &rarr; MiniGolf AST / source translator)
- **MiniGolf Compiler Frontend** (Lexer, Parser, AST, Semantic Analysis, IR Builder)
- **M6809 Backend** (Code generation, register allocation, calling conventions)
- **Runtime & Prelude** (`golflib/prelude.golf`)

---

## 1. M6809 Backend Code Generation Bugs

### 1.1 Register `Y` Clobbered by Multi-Byte Copies / `__memcpy` Without Callee-Save in Prologue [RESOLVED]
- **Subsystem**: `m6809/backend.go`, `m6809/regalloc.go`
- **Severity**: High (Silent data corruption / crashes)
- **Status**: Fixed
- **Description**:
  When the M6809 register allocator assigned a variable to callee-saved register `Y` across a function call, and a callee performed multi-byte copies (`emitCopy` / `__memcpy` / `InsertField` / `InsertElement`), register `Y` was used as a scratch register. However, `m6809/backend.go` previously only included `Y` in the function prologue's saved registers list if an SSA local variable in *that* function was allocated to `Y`.
- **Symptom**:
  Functions like `lexer_next` emitted `leay offset,s` to set up buffers for string constant copies, but the prologue only saved `u` instead of saving `y`. As a result, caller live values held in `Y` (such as AST node pointers in recursive descent parsers) were clobbered.
- **Fix**:
  Added `functionClobbersY` in `m6809/regalloc.go` and ensured `saveY` in `m6809/backend.go` checks `(usedRegs["y"] || b.functionClobbersY(f)) && !b.globalsAtY`. Functions using `Y` for scratch/multi-byte copies now save and restore `Y` in their prologue/epilogue.

### 1.2 16-Bit Integer Semantics on M6809 vs 32-Bit on CBE/AMD64
- **Subsystem**: Architecture-specific lowering
- **Severity**: Medium (Behavior divergence between backends)
- **Description**:
  In MiniGolf on M6809, `int` is a 16-bit signed integer (range -32,768 to 32,767). On AMD64 and CBE (host GCC), `int` is 32-bit. C code expecting 32-bit integer arithmetic will overflow on M6809.
- **Workaround**:
  Constrain integer calculations to 16-bit ranges or implement multi-word arithmetic routines.

---

## 2. C-to-Golf Translation (`ctranslator`) Bugs & Quirks

### 2.1 Keyword Collisions on C Struct Fields and Identifiers
- **Subsystem**: `ctranslator/translator.go`, `lexer/lexer.go`
- **Severity**: High (Compilation failure)
- **Description**:
  MiniGolf reserves keywords such as `type`, `var`, `func`, `range`, `struct`, `package`, `import`, `const`, `if`, `else`, `for`, `return`, `break`, `continue`, `defer`, `goto`, `pragma`, `nil`.
  In C99, `type` and `var` are valid identifier names (e.g., `int type;` in a struct, `void f(int var)`). When `ctranslator` emits `v.type` or `func f(var int)`, the MiniGolf lexer treats `type` as `token.TYPE` and `var` as `token.VAR`, triggering parser syntax errors (`expected next token to be IDENT, got TYPE instead`).
- **Workaround**:
  Rename C fields and variables so they do not collide with MiniGolf keywords (e.g., `val_type` instead of `type`, `loop_var` instead of `var`).

### 2.2 Struct Member Array Decay Generates Bogus Pointer Casts [RESOLVED]
- **Subsystem**: `ctranslator/translator.go` (`xExprNoDecay` / `IndexExpr`)
- **Severity**: Critical (Runtime segfault)
- **Status**: Fixed
- **Description**:
  `t.xExprNoDecay` previously only suppressed array-to-pointer decay for primary variable identifiers (`cc.PrimaryExpressionIdent`). When an array was a member of a struct (e.g., `s->items[i]` or `s.items[i]`), `ctranslator` fell back to `xExpr`, emitting `(*T)(s.items)[i]`.
  In MiniGolf, `(*T)(s.items)` did not take the address of the array; instead, it cast the *value of the first element* of the array to a pointer `*T`, and then indexed from that bogus pointer address, causing immediate segfaults on variable index accesses.
- **Fix**:
  Added `*cc.SelectorExpr` handling to `xExprNoDecay` in `ctranslator/translator.go` so that indexing struct array members emits `base.field[index]` directly without inserting the bogus pointer decay cast.

### 2.3 Single-Quote Character Literal Escape Stripping
- **Subsystem**: `ctranslator/translator.go`
- **Severity**: Medium (Syntax error in generated Golf code)
- **Description**:
  In C, character literals containing escaped single quotes (e.g., `'\'`') can have their backslash escape stripped by `ctranslator`, producing invalid Golf source such as `byte(''')`, which causes parse errors in the MiniGolf parser.
- **Workaround**:
  Use numeric ASCII constants instead (e.g., `if (c == 39)` instead of `if (c == '\'')`).

### 2.4 Lack of `enum` Support
- **Subsystem**: `ctranslator/translator.go`
- **Severity**: Medium (Missing language feature)
- **Description**:
  C99 `enum` declarations (`enum Foo { ... };`) are not fully converted into MiniGolf constants.
- **Workaround**:
  Use `#define` constants instead of `enum`.

### 2.5 Function Pointers and Indirect Calls Unsupported
- **Subsystem**: `ctranslator/translator.go`, `ir/builder.go`
- **Severity**: Medium (Translation failure / unsupported)
- **Description**:
  MiniGolf does not support C-style indirect function pointer calls (`(*func_ptr)(args)`). `ctranslator` emits `/* UNSUPPORTED: bare function */` or panics when encountering function pointers stored in variables or structs.
- **Workaround**:
  Use integer tags (`kind` / `type`) and explicit `switch` or `if-else` dispatch ladders.

### 2.6 Complex Post-Increment/Decrement Expressions
- **Subsystem**: `ctranslator/translator.go`
- **Severity**: Medium (Incorrect evaluation order / temporary aliasing)
- **Description**:
  Expressions embedding multiple postfix increments or decrements (e.g. `buf[i++] = *s++`) can produce incorrect evaluation order or compiler errors when lowered through `prelude.post_increment`.
- **Workaround**:
  Unroll post-increments into explicit standalone statements:
  ```c
  buf[i] = *s;
  i = i + 1;
  s = s + 1;
  ```

### 2.7 Comma Expressions in Value Positions
- **Subsystem**: `ctranslator/translator.go`
- **Severity**: Low (Unsupported syntax)
- **Description**:
  C comma expressions in value-producing positions (e.g., `x = (a, b)`) are unsupported and emit `/* UNSUPPORTED: comma-expr(...) */`.
- **Workaround**:
  Evaluate each expression in separate statement lines.

### 2.8 `union` Types Unsupported
- **Subsystem**: `ctranslator/translator.go`
- **Severity**: Medium (Unsupported feature)
- **Description**:
  C99 `union` types cannot be represented in MiniGolf and emit `/* UNSUPPORTED: union type */`.
- **Workaround**:
  Use `struct` with separate fields or raw byte arrays with typed pointer casts.

### 2.9 `switch` Fall-Through Unsupported
- **Subsystem**: `ctranslator/translator.go`
- **Severity**: Low (Semantic difference)
- **Description**:
  `ctranslator` lowers `switch` statements into an `if` / `else if` ladder. C `switch` blocks relying on fall-through between cases without `break` will not behave as intended.
- **Workaround**:
  Ensure all `case` blocks terminate with `break` or restructure using `if-else`.

---

## 3. Runtime & Allocator (`golflib/prelude.golf`) Issues

### 3.1 Free-List Coalescing Corruption in `free()` on M6809
- **Subsystem**: `golflib/prelude.golf` (`free` implementation)
- **Severity**: High (Heap corruption upon deallocation)
- **Description**:
  The circular free list allocator in `prelude.golf` performs pointer arithmetic to coalesce adjacent free blocks:
  ```golf
  if word(bp) + mul_word(bp.size, sizeof[MallocHeader]()) == word(p.next)
  ```
  On small 16-bit heaps with frequent reallocations, list insertion and boundary calculations can corrupt `freep` or block headers, causing subsequent `malloc` calls to return overlapping memory blocks.
- **Workaround**:
  For self-contained benchmarks and interpreter demos whose total dynamic memory footprint fits within the 20 KB M6809 heap, avoid calling `free()`. Memory can remain allocated arena-style without risk of corruption.

### 3.2 Freestanding Environment Without Standard C Headers
- **Subsystem**: `ctranslator` build environment
- **Severity**: Low (Expected freestanding constraint)
- **Description**:
  `ctranslator` compiles against `golflib/prelude.golf`. Host libc headers (`<stdio.h>`, `<stdlib.h>`, `<string.h>`, `<ctype.h>`) are not included by default.
- **Workaround**:
  Declare external runtime primitives directly:
  ```c
  extern void* zalloc(int n);
  extern void* malloc(int n);
  extern void free(void* p);
  extern void putchar(char ch);
  ```



# FIXED?

 M6809 Backend Register Y Callee-Save Fix

  • Root Cause: In backend.go and regalloc.go, when a function performed multi-byte copies (emitCopy / __memcpy / InsertField /
  InsertElement), physical register Y was used as a scratch register. The register allocator excluded Y from SSA allocation in such
  functions, but backend.go only checked usedRegs["y"] when deciding whether to save Y in the function prologue. Consequently,
  functions like lexer_next clobbered the caller's live value in Y (e.g. left in parse_expr).
  • Fix: Added regalloc.go:29 and updated saveY in backend.go:3702 to save Y whenever (usedRegs["y"] || b.functionClobbersY(f)) &&
  !b.globalsAtY.
