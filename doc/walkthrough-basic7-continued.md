# Session Walkthrough: `basic7.c` → MiniGolf Pipeline

## Current Status

| Backend | Compiles | Runs | Output |
|---------|----------|------|--------|
| CBE | ✅ | ✅ (no crash) | ❌ garbled strings |
| x86_64 | ✅ | ❌ (unknown) | — |
| M6809 | ❌ (assertion) | — | — |

**All tests pass EXCEPT `basic7` and `test_smap` (pre-existing).**

---

## Bugs Fixed This Session

### IR Builder
1. **`isConstantExpr` + `*ast.ArrayType`**: Array composite literals `[N]T{...}` now recognized as constant
2. **`isConstantExpr` + `*ast.PointerType`**: Parser represents `[N]*T{...}` as `ArrayType{Elt: PointerType{Elt: CompositeLit}}` — recursion through PointerType now reaches the CompositeLit
3. **`evalConstantExpr` ArrayType → PointerType → CompositeLit unwrap**: The body of `[N]*T{...}` is found by unwrapping PointerType layers
4. **`evalConstantExpr` element type resolution**: Replaced fragile `Sscanf("[%d]%s", ...)` parsing with `targetTyp.ArrayLength()` and `targetTyp.ArrayElementType()` — fixes `*byte` element type which was previously unresolvable as a string identifier
5. **`evalConstantExpr` string literal → pointer target type**: String literals with `*byte` target now produce `AddressOfGlobal` instead of a `slice_byte` struct

### CBE Backend  
6. **`formatVal` for `AddressOfGlobal` on array globals**: When the global is `[N]byte` but the instruction type is `*byte`, emit `&v_name.data[0]` instead of `&v_name` — gives the correct `byte*` pointer instead of a `struct*`

### x86_64 + M6809 Backends
7. **`emitData` for `ConstArray`**: Both backends now support `ConstArray` values in global initializers, enabling constant array initialization in the data section

### C Translator
8. **Variadic arg multi-dim array decay**: When a multi-dimensional array element (e.g., `stringVariables[i]` from `byte[26][64]`) is passed to a variadic function, detect that cc/v5 has already decayed the type and apply explicit `(*byte)(...)` cast

---

## Current Blocking Issue: Double `post_increment` Evaluation

### Symptom
The `printf("? %s\n", msg)` call in `printError` outputs every other character of "Undefined Line Number" → `neie ieNme`. 

### Root Cause
In [core_vsprintf at line 120](file:///home/strick/antig/c-tests/basic7.c) of the Golf output:
```golf
out_cb(*(post_increment[*byte](&s_6)), ctx)
```

The IR builder generates **two** calls to `f_prelude__post_increment_P__byte` for this single expression:
```c
v26 = f_prelude__post_increment_P__byte(v194);  // v207 — first call
v26 = f_prelude__post_increment_P__byte(v194);  // v212 — second call (overwrites)
v3 = (*v26);                                       // reads from twice-incremented pointer
```

This is a **MiniGolf compiler bug** — the expression `*(call_returning_ptr())` evaluates the call twice: once during address computation and once during value load. The pointer advances by 2 bytes per iteration instead of 1, printing every other character.

### Impact
- Affects ALL `*(post_increment[*byte](&ptr))` patterns
- The `print_str` function in prelude has the same pattern and is also affected
- The `strncmp` function uses `*(post_increment[*byte](&s2))` similarly

### Suggested Fix
The IR builder's handling of `*(call_expr)` (dereference of a call result) likely builds the call twice — once to get an address for the `LoadPtr` and once for the actual value. The fix should ensure the call is evaluated exactly once and its result is used for the dereference.

### Where to Look
The `buildExpr` function in [ir/builder.go](file:///home/strick/antig/ir/builder.go) — specifically the `PrefixExpression` handler for `*` (dereference) when the operand is a `CallExpression`.

---

## Why "Undefined Line Number"?

The BASIC interpreter fails because `strncmp` is also affected by the double `post_increment` bug. When comparing line numbers or parsing commands, `strncmp` skips every other character, causing `LET`, `DIM`, `FOR`, etc. to fail to match. The program lines are stored but cannot be executed because the line number parsing is broken.

---

## Other Outstanding Issues

| Issue | Status |
|-------|--------|
| M6809 assertion on variadic `[N]prelude.any` | Not investigated |
| `test_smap.golf` failure | Pre-existing |
| `doc/minigolf_lang.md` updates | Not started |

## Key Files Modified

| File | Changes |
|------|---------|
| [ir/builder.go](file:///home/strick/antig/ir/builder.go) | `isConstantExpr` + `evalConstantExpr` fixes for ArrayType/PointerType/CompositeLit and string→pointer constant evaluation |
| [cbe/cbe.go](file:///home/strick/antig/cbe/cbe.go) | `formatVal` AddressOfGlobal fix for array globals, `word_to_ptr` and Call arg array handling |
| [x86_64/backend.go](file:///home/strick/antig/x86_64/backend.go) | `emitData` ConstArray support |
| [m6809/backend.go](file:///home/strick/antig/m6809/backend.go) | `emitData` ConstArray support |
| [ctranslator/translator.go](file:///home/strick/antig/ctranslator/translator.go) | Variadic arg multi-dim array decay |
