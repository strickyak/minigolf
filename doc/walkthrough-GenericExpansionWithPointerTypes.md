# Fix: Generic Expansion with Pointer Type Arguments

## Problem

Instantiating a generic function with a pointer type argument (e.g., `peek[*byte]`) caused a compiler crash:
```
panic: PointedType called on non-pointer type: byte
```

The root cause was in `substituteGenericTokens` — both the IR builder and the semantic analyzer performed generic type parameter substitution at the **lexer token level**. When `T` was replaced with `*byte` (two tokens: `*` and `byte`), adjacent `*` operators in the template body would merge with the substituted `*`, creating ambiguous token sequences like `* * byte` that the parser misinterpreted.

## Changes Made

### 1. Parenthesized Token Wrapping (core fix)

#### [builder.go](file:///home/strick/antig/ir/builder.go#L363-L371)
Added logic to wrap multi-token type arguments in parentheses during token substitution. This prevents the parser from merging adjacent operators with the substituted type tokens.

#### [semantic.go](file:///home/strick/antig/semantic/semantic.go#L855-L899)
Rewrote `substituteGenericTokens` to properly lex type argument strings into tokens (instead of jamming multi-character strings into a single IDENT token's `.Literal` field), then wrap multi-token results in parentheses. Also added the `lexer` import.

### 2. Instance Name Sanitization (secondary fix)

The parenthesized token fix exposed a second bug: generic instance function names like `main.peek_*word` contained raw `*` characters, which are illegal in assembly labels.

#### [builder.go](file:///home/strick/antig/ir/builder.go#L1942) and [line 1977](file:///home/strick/antig/ir/builder.go#L1977)
Changed `instTypStr += "_" + argTyp.Name` to `instTypStr += "_" + strings.ReplaceAll(argTyp.Name, "*", "P")`.

#### [semantic.go](file:///home/strick/antig/semantic/semantic.go#L555) and [line 1137](file:///home/strick/antig/semantic/semantic.go#L1137)
Same sanitization applied to `a.exprToString(...)` results used in instance name construction.

### 3. New Test

#### [test_generic_ptr.golf](file:///home/strick/antig/tests/test_generic_ptr.golf) and [test_generic_ptr.want](file:///home/strick/antig/tests/test_generic_ptr.want)
Exercises `peek[*word]` — a generic function instantiated with a pointer type argument.

## Verification

- `test_generic_ptr.golf` passes on all 3 backends (CBE, x86_64, m6809)
- All existing tests continue to pass (failures in full suite run are pre-existing flaky M6809 emulator issues — they pass when run individually)
