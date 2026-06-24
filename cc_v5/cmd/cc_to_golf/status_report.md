# cc_to_golf — Status Report
_Written: 2026-06-24_

## What Has Been Accomplished

A fresh, comprehensive C-to-MiniGolf translator lives at
`cc_v5/cmd/cc_to_golf/cc_to_golf.go`.  It replaces the earlier prototype.

### Key features implemented

- **`-k` flag** — toggles between `panic("unsupported: ...")` (default) and
  `/* UNSUPPORTED: ... */` comments for constructs we cannot yet handle.

- **Full typed-AST expression walker** — covers:
  - `BinaryExpression`, `AssignmentExpression`
  - `SelectorExpr` — both `.` and `->` become `.` (MiniGolf auto-deref)
  - `CallExpr` — arguments collected in source order (first→last); an earlier
    incorrect `reverse()` call was removed after discovering that
    `ArgumentExpressionList` in cc_v5 follows the chain first→last.
  - `CastExpr` — primitive types become `T(expr)`, pointer types become
    `(*T)(expr)` (parentheses prevent misparse as double-dereference).
  - `IndexExpr`, `UnaryExpr`, `PostfixExpr`, `PrefixExpr`
  - `ConditionalExpression` — ternary hoisted to a temp var (no-`-k`),
    or emitted as a comment (with `-k`).

- **Struct / typedef pre-scan** — maps `typedef struct bin { ... } *Bin` to a
  MiniGolf `type Bin struct { ... }` definition and enters `"Bin" → "*Bin"`
  in the type map so parameters and field references come out right.

- **Static local extraction** — e.g. `static struct bin a` inside `Traverse`
  becomes the global `var _Traverse_a Bin`.  All references to `a` inside
  that function are automatically substituted to `_Traverse_a` via a
  per-function `staticNameMap`.

- **Argument order** — `cc_v5`'s `ArgumentExpressionList` linked list is in
  source order; the earlier `reverse()` call was wrong and has been removed.

- **Pointer-cast parentheses** — `*(volatile byte*)0xff00` translates to
  `*(*byte)(0xff00)` (not the broken `**byte(0xff00)` from before).

- **Infinite-loop detection** — `while (1)` → `for {`.

- **`do { } while (0)` macros** — body is emitted directly (no loop).

- **All statement types** — if / else-if chains, switch/case/default,
  for / while / do-while, for-with-decl, break / continue / return,
  labeled statements, goto → unsupported comment.

### Verified output

Running:
```
go run cc_v5/cmd/cc_to_golf/cc_to_golf.go -k c-demos/collatz-bignum/_whole-collatz.c_ > _tmp/wc.golf
```
produces recognisable MiniGolf with correct structure, correct argument order,
and correctly mangled static globals throughout.

---

## What Is Broken / Known Issues

### 🔴 High priority (blocks compilation)

1. **`for`-init-decl pointer-type cast** — e.g. `const char* s = format` in a
   `for` initialiser generates `s := *byte(format)` instead of
   `s := (*byte)(format)`.  A `castInit()` helper was being wired up when work
   stopped.  See `forInitDecl()` in the source — the two `Sprintf` calls with
   `"%s := %s(%s)"` need to use `castInit(golfType, init)` instead.

2. **Pointer arithmetic as statements** — `s++` / `p++` where `s` or `p` is a
   `*byte` appears in the `printf` helper functions.  MiniGolf has no pointer
   increment.  These need to become:
   ```golf
   s = (*byte)(word(s) + 1)
   ```
   Detection: in `translateExprStmt` / statement-level `PostfixExpr`, check
   whether the operand is a pointer type and emit the word-cast increment
   instead of bare `++`.

3. **`*p++ = expr` split** — assignment where the LHS is `*(p++)`.  Needs to
   become two statements:
   ```golf
   *p = expr
   p = (*byte)(word(p) + 1)
   ```
   Detection: in `translateExprStmt`, check for `AssignmentExpression` whose
   LHS is `UnaryExpr(Deref, PostfixExpr(++))`.

### 🟡 Medium priority (wrong output, not crash)

4. **Ternary placeholder** — with `-k`, ternaries become
   `/* TERNARY(cond) ? (a) : (b) */0`.  The trailing `0` is a lie; it should
   at least be an obvious marker like `/* TERNARY_TODO */`.

5. **`va_list` / `va_arg`** — The `Vprintf` / `printf` implementation uses
   varargs internals.  MiniGolf cannot express this.  For the collatz program,
   the right approach is probably to hand-replace the C `printf` calls with
   MiniGolf's `print` / `println` builtins rather than attempting to translate
   the varargs machinery.

6. **`for (; *s; s++)` syntax** — produces `for ; *s; s++ {` with an
   unbalanced leading `;`.  The init part should be dropped when empty.

### 🟢 Low priority / cleanup

7. **Dead `reverse()` function** — still in the file, never called.  Remove.

8. **`while (1)` variants** — `while (1 != 0)` or `while (true)` are not
   detected as infinite loops (only the literal `1` is).  Could broaden.

9. **`_whole-collatz.c_` output does not yet compile** — because of items 1–3
   above.  The bignum logic itself (bin.c / collatz.c portions) translates
   cleanly; the problem is confined to the printf helpers.

---

## What Is Left To Do

| Priority | Task |
|---|---|
| 🔴 | Add `castInit(golfType, init) string` helper that wraps pointer types in `(...)` and call it from `forInitDecl` |
| 🔴 | In `translateExprStmt`, detect pointer-type `PostfixExpr` (`s++` / `p++`) and emit `s = (*byte)(word(s)+1)` |
| 🔴 | In `translateExprStmt`, detect `*(p++) = expr` and split into two statements |
| 🟡 | Replace ternary `-k` placeholder `0` with a less misleading token |
| 🟡 | Fix empty-init `for ; cond; post {` → drop the leading `;` |
| 🟡 | Remove dead `reverse()` function |
| 🟢 | Verify the collatz logic compiles with `sh run9.sh _tmp/wc.golf` |
| 🟢 | Consider a hand-written MiniGolf `printf` shim that wraps `print`/`println` |

---

## How to Run

From the repo root:

```sh
# Best-effort translation (comments on unsupported constructs):
go run cc_v5/cmd/cc_to_golf/cc_to_golf.go -k c-demos/collatz-bignum/_whole-collatz.c_ > _tmp/wc.golf

# Strict translation (panics on unsupported constructs):
go run cc_v5/cmd/cc_to_golf/cc_to_golf.go    c-demos/collatz-bignum/_whole-collatz.c_ > _tmp/wc.golf

# Compile and run on 6809 emulator:
sh run9.sh _tmp/wc.golf

# Compile and run on all 3 backends:
sh run4.sh _tmp/wc.golf
```

## Relevant Files

| File | Purpose |
|---|---|
| `cc_v5/cmd/cc_to_golf/cc_to_golf.go` | The translator (this session's main work) |
| `c-demos/collatz-bignum/_whole-collatz.c_` | Pre-processed C source to translate |
| `_tmp/wc.golf` | Last generated output (not checked in) |
| `cc_v5/ast.go` | cc_v5 AST struct definitions (reference) |
| `cc_v5/type.go` | cc_v5 Type interface and StructType methods |
| `doc/minigolf_lang.md` | MiniGolf language reference |
| `golflib/big.golf`, `golflib/mem.golf` | MiniGolf struct/pointer examples |
