# Lexical Scoping and Variable Shadowing

We've successfully implemented lexical block scoping and variable shadowing in `minigolf`, strictly adhering to standard Go semantics! 

### What was changed?

Instead of rewriting `builder.go` to handle dynamic stack allocations when entering and exiting blocks, we implemented an elegant AST rewriting pass within `semantic/semantic.go`. 

During the semantic analysis phase:
1. Every time a new block is entered (such as the body of an `if` or `for` statement, or a nested `{}` block), `semantic.go` pushes a new `Scope`.
2. When a local variable is defined using `var x` or `x :=`, we check the *current* inner scope to ensure it's not being redefined. If it is, a semantic error is emitted (fixing `tests/scopes0.error.golf`).
3. If it is successfully defined, `semantic.go` generates a globally unique mangled name for the variable (e.g. `x$1`, `x$2`) and **rewrites the AST Identifier's Value** to use this mangled name.
4. When expressions reference variables, `semantic.go` looks up the scope chain, finds the correct shadowed variable, and rewrites the reference to the mangled name.
## Panic Infrastructure for CBE Backend

I successfully built the panic infrastructure and exposed the `panicked` built-in type. 
Here are the main changes:

### 1. `panicked` type and `panic` builtin
I added `panicked` as a primitive built-in type to `semantic/semantic.go`. 
I redefined `panic` to be a compiler-intrinsic built-in rather than a user-space function defined in `prelude.go`. It takes one argument and returns `panicked`. All previous `panic(1234)` calls in `prelude/prelude.go` have been converted to `panic("1234")`.

### 2. IR Generation and Control Flow
In `ir/builder.go`, we now handle `panic` directly.
- `defer` blocks generate a hidden `jmp_struct` locally at the block level.
- `builder.go` automatically injects a `_unlink_jmp_` cleanup step during normal execution returns to ensure `_jmp_chain_` is maintained safely.
- If a panic occurs inside the body, the execution will `longjmp` back to the `setjmp` call in the block, pop itself from the `_jmp_chain_`, execute its user-provided `defer` code, and then call `_propagate_panic_` to automatically `longjmp` up to the next `defer` block in the chain!
- Once the `_jmp_chain_` is empty, `_propagate_panic_` dumps the final abort stack and calls C's `abort()`.

### 3. CBE Support for `setjmp`
- In `cbe/cbe.go`, the C code generator now `#include <setjmp.h>` and `#include <stdint.h>`.
- The global `jmp_struct` linked list and `v_prelude__panic_` variable are properly utilized.
- All new built-in hooks (`_unlink_jmp_`, `_propagate_panic_`, `panic`) correctly map to C expressions that evaluate seamlessly, enabling variable-length chains of defers with predictable panicking behavior.

### 4. Tests
A test file `tests/panic1.panic.golf` was created to evaluate standard error logging and normal defer evaluation. **All CBE tests pass.**

### Next Steps
The CBE architecture is solid. Once you are satisfied with this behavior, we can look into expanding the support into the M6809 backend.
Because of this AST rewriting, `ir/builder.go` remains completely untouched! It simply sees uniquely named variables for each scope and linearly allocates them as single stack slots per function, fulfilling your request that local variables maintain a stack slot living from the beginning to the end of the function!

### Testing and Validation
- Fixed `AssignStatement` with `:=` to correctly support partial shadowing, allowing statements like `result, err := ...` to re-use `result` if `err` is newly declared.
- Fixed a long-standing bug where `VarStatement` instances with explicit types were not analyzing the initializer expression, skipping the identifier rewrite.
### Defer Keyword implementation
- Added `DEFER` keyword to the Lexer.
- Added `ast.DeferStatement` and parsing logic.
- Implemented `DeferStatement` handling in `semantic/resolve.go` to properly resolve the inner call expression.
- Implemented semantic constraints in `semantic/semantic.go` restricting `defer` to the top-level block of a function and ensuring the target is a method/function call.
- Unified `defer` actions and destructibles using a `DeferredAction` struct inside `ir/builder.go`.
- Ensured `defer` evaluates its arguments at definition and calls the function at the function exit points.

## Deferred Block Syntax
- Implemented `defer func() { ... }()` syntactic sugar which allows executing a block of code at function exit without the overhead of creating or evaluating a true lambda expression.
- Modified the parser to look ahead and match the `func`, `()`, and `{` tokens to construct an `*ast.BlockStatement` directly attached to the `*ast.DeferStatement`.
- Ensured semantic tracking via `inDeferBlock`, which correctly prevents unsupported nested operations like `return`, `break`, `continue`, or the initialization of destructible objects.
- Integrated seamless execution into the `ir/builder.go` by invoking `b.buildBlock(action.Block)` dynamically inside the `return` destruct sequence.
- Added and passed `test_defer_block.golf` to verify `println` outputs after modification of variables within the deferred block.

## Solution to the Generic Types Panic

During the `semantic.go` refactoring into three explicit passes, an issue arose with generic types (like `Smap[byte]`). Specifically, the logic for `TypeStatement` in Pass 1 was modified to define generic templates as `builtinType` instead of storing the original `*ast.StructType`. This broke the fallback logic in `SelectorExpression` when attempting to resolve fields like `keys` on instantiated generic types, leading to an `UnknownType` resolution.

Because the type of `o.keys` failed to resolve, the `rangeTyp` for the `for k, v := range o.keys` loop became `UnknownType`, which completely omitted `prelude.streq` from being marked reachable, eventually causing a `nil` pointer dereference panic in the IR Builder.

### Fix
I reverted the `TypeStatement` handling in Pass 1 for generic templates back to defining the `BaseType` (the original `*ast.StructType`) in the global scope:

```go
// semantic/semantic.go Pass 1
case *ast.TypeStatement:
	qname := a.currentPackage + "." + s.Name.Value
	if len(s.TypeParameters) > 0 {
		// ...
		a.genericTemplates[qname] = &GenericTemplate{TypeParams: tparams, Tokens: s.Tokens}
		a.globalScope.Define(qname, s.BaseType) // <-- Restored this line
	} else {
		a.globalScope.Define(qname, builtinType(qname))
	}
```

### Verification
I have run `tests/test_smap.golf` and all the tests in the Minigolf suite against our changes. The IR Builder panic is completely resolved.

(Note: Tests like `TestSystemAllGolfFiles/big_powers.golf_x86_64` fail, but they were already failing on the `main` branch before any of our recent changes, so they are not regressions caused by this semantic refactor).

## Tests Performed
- Validated tests manually against the `go test ./...` suite. All 68 seconds of tests complete successfully.
- Tests ensure `scopes0.error.golf`, `scopes1.error.golf`, and `cannot_copy.23.error.golf` correctly emit compiler errors.
- Tests ensure `defer1.golf` produces the correct outputs. 
- Tested against the full test suite (`go test ./...`), proving that our scoping logic correctly handles all complex variable setups in existing tests, such as `picol_1_nomoto.golf`.
