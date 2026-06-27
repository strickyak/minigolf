# Session Walkthrough: `basic7.c` Double-Evaluation Bug Fix

## Final Test Results

| Test | Before | After |
|------|--------|-------|
| `basic7/CBE` | ❌ garbled output | ✅ PASS |
| `basic7/x86_64` | ❌ garbled/crash | ✅ PASS |
| `basic7/m6809` | ❌ assertion | ❌ (pre-existing) |
| `array_iter` (x86/m6809) | ❌ regression | ✅ PASS |
| `big_powers` (x86/m6809) | ❌ regression | ✅ PASS |
| `init_val2` (x86/m6809) | ❌ regression | ✅ PASS |
| `test_smap` (CBE) | ❌ | ❌ (pre-existing) |
| All other tests | ✅ | ✅ |

---

## Root Cause: Double-Evaluation Bug

### The Bug

In [ir/builder.go](file:///home/strick/antig/ir/builder.go), the `buildCall` function for identifier-based calls had this flow:

```
1. Line 2314-2316: Build all arguments (emitting IR with side effects)
2. Line 2318-2326: Resolve function name
3. Line 2327: f = b.funcs[funcName] → nil (not a named function)
4. Line 2339-2346: Indirect call fallback → BUILD ARGUMENTS AGAIN
```

When a variable (like `out_cb`) was used as a function pointer, the arguments were built twice: once at step 1 (for the function resolution path) and again at step 4 (for the indirect call fallback). The first set of args was discarded but their side effects (like `post_increment`) persisted.

### Impact

Any expression with side effects passed as an argument to an indirect call through a variable would be evaluated twice:

```golf
out_cb(*(post_increment[*byte](&s_6)), ctx)  // post_increment called TWICE
```

This caused:
- String printing (`core_vsprintf %s`) to skip every other character
- `strncmp` to compare wrong characters
- The BASIC interpreter's error message "Undefined Line Number" to appear as `neie ieNme`

### The Fix

[ir/builder.go:2339-2345](file:///home/strick/antig/ir/builder.go#L2339-L2345): Reuse the `args` already built at line 2314 instead of rebuilding them in the indirect call fallback.

```diff
 // It's not a typedef, treat as an indirect call from a variable holding a function!
 funcVal := b.buildExpr(e.Function)
 if b.CheckNil {
     b.emitNilCheck(funcVal, e)
 }
-var args []Value
-for _, arg := range e.Arguments {
-    args = append(args, b.buildExpr(arg))
-}
+// Reuse args already built above (line 2314-2316) to avoid
+// double-evaluating expressions with side effects (e.g.
+// post_increment calls).
```

---

## All Bugs Fixed (9 total)

### 1. Double-evaluation in indirect call args (IR Builder)
- **File**: [ir/builder.go](file:///home/strick/antig/ir/builder.go#L2339-L2345)
- **Impact**: All `*(post_increment[*byte](&ptr))` patterns doubled, corrupting string operations

### 2-5. Global array-of-pointer initialization (IR Builder)
- **File**: [ir/builder.go](file:///home/strick/antig/ir/builder.go)
- `isConstantExpr` + `*ast.ArrayType` recursion
- `isConstantExpr` + `*ast.PointerType` recursion  
- `evalConstantExpr` pointer-to-CompositeLit unwrap
- `evalConstantExpr` `ArrayElementType()`/`ArrayLength()` replacing fragile string parsing

### 6. AddressOfGlobal for array-backed strings (CBE)
- **File**: [cbe/cbe.go](file:///home/strick/antig/cbe/cbe.go#L464-L472)
- Emit `&v_name.data[0]` instead of `&v_name` when global is `[N]byte` but instruction type is `*byte`

### 7-8. ConstArray support in emitData (x86_64 + M6809)
- **Files**: [x86_64/backend.go](file:///home/strick/antig/x86_64/backend.go#L1188-L1191), [m6809/backend.go](file:///home/strick/antig/m6809/backend.go#L2278-L2281)
- Both backends now handle `*ir.ConstArray` in global data emission

### 9. Variadic arg multi-dim array decay (C Translator)
- **File**: [ctranslator/translator.go](file:///home/strick/antig/ctranslator/translator.go#L1591-L1613)
- Detect when cc/v5 has already decayed an `IndexExpr` on a multi-dimensional array

---

## Remaining Pre-existing Issues

| Issue | Description |
|-------|-------------|
| `basic7/m6809` | M6809 backend assertion on variadic `[N]prelude.any` slices |
| `test_smap.golf_CBE` | Pre-existing failure, not investigated |
