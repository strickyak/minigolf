# Add Support for Named Return Variables

This proposal outlines the required changes across the compiler frontend and intermediate representation (IR) to support named return variables (e.g., `func Open(filename string) (fd *File, ok bool)`). 
It also covers the semantics of a simple "naked" `return` (without arguments) and how explicit returns will override these variables. This mechanism is crucial for proper `panic` recovery and deferred execution.

## User Review Required

> [!IMPORTANT]
> **Parser Strategy for Return Parameters**: Go allows both `(int, string)` and `(a int, b string)` for return signatures. To distinguish between a type and a named parameter in the parser without backtracking, we can parse an expression; if it's an identifier and the next token is not a comma or parenthesis, we treat it as a name and parse the subsequent expression as its type. Does this parsing strategy align with how you want MiniGolf to handle function signatures?

> [!IMPORTANT]
> **Naked Return Implementation**: I propose handling naked returns in the **IR Builder** rather than rewriting the AST during Semantic Analysis. The IR Builder will maintain stack slots for named return variables (zero-initialized at function start). On encountering a naked `return`, the Builder will simply load from these slots. If an explicit `return value1, value2` is encountered, the Builder will store the values into the named return slots before emitting the IR return instruction. Does this architecture seem correct?

## Open Questions
None at the moment.

## Proposed Changes

---

### AST Layer

#### [MODIFY] [ast/ast.go](file:///home/strick/antig/ast/ast.go)
- Change `ReturnTypes []Expression` to `ReturnParameters []*Parameter` in both `FuncStatement` and `FuncType`.
- `*Parameter` already supports an optional `Name` and a `Type`, perfectly modeling both named and anonymous return values.

#### [MODIFY] [ast/printer.go](file:///home/strick/antig/ast/printer.go)
- Update the AST printer to iterate over `ReturnParameters` instead of `ReturnTypes` when formatting `FuncStatement` and `FuncType`.

---

### Parser Layer

#### [MODIFY] [parser/parser.go](file:///home/strick/antig/parser/parser.go)
- Update the `func` signature parsing block. 
- Introduce a new helper `parseReturnParameters()` that gracefully handles both anonymous types and named parameters enclosed in parentheses.
- If a single anonymous return type is provided without parentheses (e.g., `func f() int`), parse it as a single `*Parameter` with no name.

---

### Semantic Analysis Layer

#### [MODIFY] [semantic/semantic.go](file:///home/strick/antig/semantic/semantic.go)
- **Scope Registration**: In the `FuncStatement` visitor, iterate over `ReturnParameters`. If a parameter has a `Name`, register it as a local variable in the function's top-level scope so that the function body can resolve references to it.
- **Type Checking**: Update type inference logic for return statements to check against `ReturnParameters` instead of `ReturnTypes`.
- Ensure that if a function has named return variables, a naked `return` (where `ReturnValues` is empty) is marked as valid.

#### [MODIFY] [semantic/resolve.go](file:///home/strick/antig/semantic/resolve.go)
- Update `resolveExpression` paths for `FuncType` and `FuncStatement` to iterate over `ReturnParameters` to recursively resolve their underlying type expressions.

---

### IR Builder Layer

#### [MODIFY] [ir/builder.go](file:///home/strick/antig/ir/builder.go)
- **Variable Allocation**: In `buildFunction`, check for named return parameters. For each one, allocate a local stack slot/variable exactly as is done for normal function parameters or `var` statements, and initialize them to their zero values.
- **Return Handling**: 
  - When encountering an `*ast.ReturnStatement` with **no arguments** (naked return): Load the current values from the named return variable slots, and emit them as the return values.
  - When encountering an `*ast.ReturnStatement` with **explicit arguments**: Evaluate the arguments, store their values into the named return variable slots (so that any subsequent `defer` blocks see the updated values), and then return.
- **Helper Updates**: Update methods like `getFuncReturnType()` to map `ReturnParameters` to an `ir.Type` (likely an `ir.StructType` if multiple returns are used).

## Verification Plan

### Automated Tests
- Write a new MiniGolf test file (e.g., `tests/named_returns.golf`) that declares a function with named returns, assigns to them, uses a naked return, and verifies the correct outputs.
- Write a second test function in the same file that explicitly returns values to ensure the named variables are correctly overridden.
- Run `go test ./...` to verify the frontend and backend properly lower the new AST constructs into functional assembly code.
