# Fix: Generic Expansion with Pointer Type Arguments

## Problem 1: Pointer type args crash the compiler

Instantiating a generic function with a pointer type argument (e.g., `peek[*byte]`) caused:
```
panic: PointedType called on non-pointer type: byte
```

**Root cause:** `substituteGenericTokens` in both `ir/builder.go` and `semantic/semantic.go` does token-level substitution. When `T` is replaced with `*byte` (tokens: `*`, `byte`), adjacent `*` operators merge, creating `* * byte` which the parser misinterprets.

## Problem 2: Qualified type names break var declarations

Initial fix (wrapping ALL multi-token args in parens) caused a new failure:
```
panic: variable declaration without type or value
    in buildStatement: *ast.VarStatement (at smap.golf [expanded main.Lookup...]:65)
```

**Root cause:** For `Smap[string]`, the type arg resolves to `prelude.slice_byte` (3 tokens: `prelude`, `.`, `slice_byte`). Wrapping in parens produced `var zeroT (prelude.slice_byte)`, which the parser treated as a value expression, not a type.

## Solution

Only wrap in parentheses when the **first token is `*` (ASTERISK)** — the only case where token merging is ambiguous. Qualified names like `prelude.slice_byte` don't need wrapping.

Additionally, sanitize `*` → `P__` in generic instance function names to avoid illegal assembly label characters and collisions with user-defined types (e.g., `Pint`).

---

# Feature: `cond` Magic Syntax for Ternary Operators

## Problem
The `cc_to_golf` C translator encountered C ternary operators (`cond ? thn : els`), but MiniGolf has no ternary operator.

## Solution
Implemented a new magic built-in function `cond(p, y, n)` that translates into branching short-circuit logic, identical to `&&` and `||`.

### 1. Registration
In `semantic/semantic.go`, defined `cond` as `FuncTypeBuiltin` so it bypasses normal type resolution and signature checking.

### 2. IR Generation (Short-Circuiting)
In `ir/builder.go` `buildCall()`, intercepted `"cond"` calls:
- Creates three basic blocks: `trueBlk`, `falseBlk`, `endBlk`.
- Emits a conditional `Branch` instruction using the condition argument.
- Evaluates the "true" expression only in `trueBlk` and the "false" expression only in `falseBlk`.
- Emits a `Phi` instruction in `endBlk` to join the values from both branches into a single SSA value.

### 3. Type Coercion
Added support for coercing literals between branches:
- If one branch evaluates to `const_integer` or `nil`, and the other is a concrete type (like `int` or `*byte`), the literal is coerced to the concrete type.
- If one branch evaluates to `noreturn` (e.g. from `panic()`), the result type is inferred from the other branch.

### 4. Dead Branch Elimination
Because `cond` emits standard `Branch` instructions, the existing Dead Branch Elimination pass (`opt/dbe.go`) automatically folds `cond` expressions with constant conditions (e.g. `cond(true, 1, 2)`) into direct `Jump`s, completely eliminating the dead branches.

### 5. C Translator Update
Updated `cc_to_golf/cc_to_golf.go` to unconditionally translate `a ? b : c` to `cond(a, b, c)`, rather than emitting an unsupported comment block placeholder.

## Verification
- Added `tests/test_cond.golf` which tests short-circuiting (`cond(true, 1, panic())`), literal coercion (`100` -> `int`, `nil` -> `*byte`).
- `test_cond.golf` passes on all three backends (CBE, x86_64, m6809).
