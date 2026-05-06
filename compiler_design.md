# Design Document: Optimizing SSA Compiler for MiniGolf

## 1. Introduction and Goals

This document outlines the design of an optimizing compiler for a simplified, Go-like language (referred to as "MiniGolf" - MINIature GO Language, Fun!). The compiler is built around a Static Single Assignment (SSA) intermediate representation to facilitate robust and efficient optimization passes.

**Implementation Language:** The compiler will be written in **Go version 1.25**.

**Primary Goals:**
*   **Correctness:** Accurately translate the source language into the target machine code or lower-level IR.
*   **Optimization:** Implement standard scalar optimizations leveraging the properties of SSA form.
*   **Simplicity & Modularity:** Keep the compiler phases distinct and well-defined, focusing on the specific constraints of the language.

## 2. Source Language Overview

The language is a subset of Go, featuring a similar syntax and static typing.

**Module System and Compilation:**
*   **Whole Program Compilation:** The compiler operates on all source files at once. There are no separate libraries or linking phases.
*   **Flat Namespace:** Each source `.go` file becomes one module in a flat module namespace.
*   **Entry Point:** Any compilation must include a file declared as `package main`.

**Program Structure (Top-Level Statements):**
The language supports exactly six outer statement types, just like in Go:
1.  `package`: Defines the package name.
2.  `import`: Declares dependencies.
3.  `const`: Declares constants.
4.  `type`: Declares type aliases.
5.  `var`: Declares global variables.
6.  `func`: Declares functions.

**Data Types & Literals:**
*   **Primitive Integers:** Exactly two numeric types: `byte` (unsigned 8-bit) and `word` (unsigned pointer-sized integer, equivalent to `uintptr`).
*   **Composite Types:** Structs (e.g., `type Apple struct { worms byte }`) and fixed-size arrays (e.g., `[10]byte`).
*   **Pointers:** Pointers to primitives and structs (e.g., `*Apple`), generated via the address-of operator `&` and dereferenced automatically or manually.
*   **Strings:** ASCII string literals are supported strictly for testing and bootstrapping. No string operations (like concatenation) are defined; they can only be passed to built-in print functions.

**Methods:**
The language provides Go-style method declarations on user-defined pointer types.
*   `func (a *Apple) NumWorms() byte { return a.worms }`
*   Methods are statically dispatched at compile time. The receiver (`a`) acts as an implicit first parameter.

**Function-Level Statements:**
Inside functions, the following statements are permitted:
*   **Assignments:** Standard assignments and multi-value assignments.
*   **Function Calls:** Invoking user-defined or built-in functions.
*   **Control Flow:** `if` statements and `while` loops.
*   **Expressions:** Standard arithmetic and bitwise operations.

**Built-in Functions:**
For bootstrapping, the language provides:
*   `print(...)`: Prints its arguments.
*   `println(...)`: Prints its arguments followed by a newline.

## 3. Compiler Architecture

The compiler follows a standard multi-pass architecture:

1.  **Lexical Analysis (Scanner):** Converts the source text into a stream of tokens.
2.  **Syntax Analysis (Parser):** Constructs an Abstract Syntax Tree (AST) from the token stream.
3.  **Semantic Analysis & Type Checking:** Annotates the AST with type information and verifies semantic correctness, enforcing strict typing between `byte` and `word`.
4.  **IR Generation:** Translates the typed AST into the initial SSA-form Intermediate Representation.
5.  **Optimization Pipeline:** Applies a series of transformations to the SSA IR to improve performance.
6.  **Instruction Selection & Register Allocation:** Lowers the SSA IR into machine-specific instructions and assigns physical registers (or lowers to another IR like LLVM or WebAssembly).
7.  **Code Emission:** Generates the final executable or object code.

## 4. Intermediate Representation (SSA Form)

The core of the optimization pipeline relies on an SSA representation where every variable is assigned exactly once. This property vastly simplifies data-flow analysis.

### 4.1 SSA Structure

*   **Functions:** The top-level compilation unit containing a Control Flow Graph (CFG).
*   **Basic Blocks:** A sequence of instructions with a single entry point and a single exit point (terminator).
*   **Instructions:** Operations that compute values. Each instruction produces at most one value.
*   **Values:** The result of an instruction. Values are strictly typed as `byte` or `word`.

### 4.2 Key Instruction Types

The IR operations are strictly defined as Go structs implementing the `Instruction` interface:

*   **Constants**: `ConstByte`, `ConstWord`
*   **Memory Operations**: `Load`, `Store` (used for globals). `AddressOfGlobal`, `AddressOfLocal` (taking addresses). `LoadPtr`, `StorePtr` (dereferencing pointers).
*   **Struct & Array Operations**: `ExtractElement`, `InsertElement` (arrays). `ExtractField`, `InsertField` (struct values). `ExtractFieldPtr`, `InsertFieldPtr` (struct pointers).
*   **Arithmetic/Logic (`BinaryOp`, `UnaryOp`)**: Supported opcodes include `add`, `sub`, `mul`, `div`, `mod`, `and`, `or`, `xor`, `shl`, `shr`, `not`, `neg`.
*   **Comparisons (`Compare`)**: `eq`, `neq`, `lt`, `lte`, `gt`, `gte`. These always yield a `byte` (0 for false, 1 for true).
*   **SSA Primitives**: `Phi` (selects value based on predecessor block).
*   **Function Calls**: `Call` (user-defined functions), `BuiltinCall` (`print`, `println`).
*   **Conversions (`Cast`)**: `zero_ext` (byte to word), `trunc` (word to byte).
*   **Metadata**: `SourceMarker` (embeds source code line numbers and statement context for debugging comments).
*   **Terminators**: 
    *   `Jump`: Unconditional branch to a single target block.
    *   `Branch`: Conditional branch evaluated on a condition value.
    *   `Return`: Returns execution to caller, yielding an optional value.

## 5. Type System Details (Byte and Word)

The strictness of having only `byte` and `word` requires careful handling during AST-to-IR translation and optimization.

*   **Arithmetic Rules:** Operations generally require both operands to be of the exact same type. The result is of the same type. 
*   **Overflow Behavior:** Overflow behaves according to standard unsigned integer arithmetic modulo $2^8$ (for `byte`) or $2^{16}$ (for `word`).
*   **Casting:** Explicit casts are necessary for mixing types in the source language.
    *   `word(b)`: Translates to a `ZeroExt` instruction in IR.
    *   `byte(w)`: Translates to a `Truncate` instruction in IR.
*   **Comparisons:** `Eq`, `Neq`, `Lt`, `Lte`, `Gt`, `Gte` require operands of the same type. The result of a comparison is conceptually a boolean, but can be represented internally as a `byte` (0 for false, 1 for true) to minimize primitives.

## 6. Optimization Passes

The SSA form enables several classical and powerful optimizations:

1.  **Constant Folding:** Evaluates expressions with known constant values at compile time (e.g., replacing `Add_Byte(2, 3)` with `5`).
2.  **Constant Propagation:** Replaces uses of a value with a constant if it is known to always hold that constant.
3.  **Copy Propagation:** Eliminates redundant copies of values (e.g., replacing `x = y; z = x` with `z = y`).
4.  **Dead Code Elimination (DCE):** Removes instructions whose results are never used and that have no side effects.
5.  **Common Subexpression Elimination (CSE):** Identifies and merges identical calculations within a basic block or globally (GVN).
6.  **Strength Reduction:** Replaces expensive operations with cheaper ones (e.g., replacing multiplication by a power of two with a left shift: `Mul_Word(x, 4)` -> `Shl_Word(x, 2)`).
7.  **Peephole Optimizations:** Local optimizations looking for specific small patterns (e.g., `Truncate(ZeroExt(byte_val))` -> `byte_val`).

*Execution Strategy:* These passes are typically run iteratively in a loop until a "fixed point" is reached, meaning a full pass over the IR results in no further changes.

## 7. Target Code Generation Strategy

The target code generation phase is designed to be pluggable and extensible. The strict use of 8-bit (`byte`) and 16-bit (`word`) primitive types makes the language particularly well-suited for classic 8-bit and 16-bit microprocessors.

### 7.1 Primary Targets: Motorola 6809 and Hitachi 6309
The primary focus for target architecture is the **Motorola 6809** and the **Hitachi 6309**.
*   **Motorola 6809:** An advanced 8-bit processor with powerful 16-bit capabilities (e.g., D, X, Y, U, S registers). The `byte` and `word` types map cleanly to its register set and instruction semantics.
*   **Hitachi 6309:** A compatible but enhanced version of the 6809. The compiler will optionally leverage its additional registers (e.g., W, V, Q) and extra instructions (like hardware division, extended math, and block moves) to produce highly optimized machine code.

#### 6809 Calling Convention & ABI
To seamlessly link with existing C libraries built by `gcc` v4.6.4 (targeting `lwasm`), the backend strictly adheres to the GCC 6809 ABI:
*   **Parameter Passing**: Arguments are evaluated and pushed to the hardware stack `S` in reverse order (right-to-left). However, the *first* `word`-sized argument is intercepted and passed in the `X` register, and the *first* `byte`-sized argument is passed in the `B` register.
*   **Varargs**: When a function accepts variadic arguments (like `printf`), the last named variable prior to `...` *must* be pushed onto the stack instead of its usual register, ensuring the `va_list` library can compute physical memory offsets linearly.
*   **Return Values**: Single return values are returned cleanly in hardware registers: `word`-sized values in `X` and `byte`-sized values in `B`.

#### 6809 Stack Management (S and U Registers)
The 6809 backend utilizes a strict dual-register stack model to map infinite SSA variables to physical memory while conforming to the ABI constraints:
*   **Hardware Stack (`S`)**: Exclusively reserved for passing parameters to external function calls and handling `jsr`/`rts` return addresses. During mathematical expression evaluation, it serves as a temporary scratchpad (e.g., operands are dynamically pushed via `std ,--s` and instantly consumed via `addd ,s++`).
*   **User/Frame Stack (`U`)**: Re-assigned as the local function frame pointer. Upon entry, the compiler issues `pshs u` and `tfr s,u` to establish a fixed memory frame. Every SSA instruction dynamically receives a 2-byte local stack slot accessed via negative frame offsets (`-offset,u`). To unify parameter reads and eliminate register tracking complexities, ABI parameters arriving natively in `X` and `B` are immediately copied down into local `U` slots during the function prologue.

#### 6809 Execution Mode Flags
To support deployment across advanced operating systems (such as Microware OS-9 or NitrOS-9) and to provide deep flexibility over register allocation, the M6809 backend supports three target-specific mode flags:
*   **`-frame-pointer=bool` (default `false`)**: Toggles the use of the `U` register as a dedicated local frame pointer. When `false`, the compiler dynamically tracks physical byte-pushes against the hardware `S` stack to emit differential frame queries, completely freeing `U` for usage within the local basic-block register allocator.
*   **`-globals-at-y=bool` (default `false`)**: When enabled, the backend maps all global variable references as contiguous relative offsets against the `Y` register (`N,y`), avoiding hardcoded `.data`/`.bss` locations. The system masks `Y` out of standard register allocation workflows, allowing it to act securely as a constant local bounds pointer.
*   **`-pic=bool` (default `false`)**: Triggers strict Position Independent Code generation. Formats all standard procedural leaps (`jsr`) into relative branching vectors (`lbsr`). Systematically captures string configurations and read-only elements, nesting them purely within the continuous `.code` execution section to ensure `leax fmt,pcr` addressing logic natively bypasses OS partition disconnects.

### 7.2 The C Transpiler and CBE Backend
To bootstrap the language and provide a highly portable compilation path, the system includes two C generators:
*   **C Transpiler (`-m=C`)**: Transpiles the parsed AST directly into equivalent high-level C code. This circumvents the SSA IR entirely, generating readable C structs, pointers, and variables that closely mirror the original MiniGolf source code. 
*   **CBE (C Backend from SSA, `-m=CBE`)**: Translates the optimized, lowered SSA IR pipeline into C code. Variables are emitted as flat `v1`, `v2`, `v3` SSA variables alongside explicit block labels (`.Lb1`) and `goto` jumps.

### 7.3 Testing and Debugging Target: x86_64
For practical debugging, testing, and rapid development, the compiler includes a native machine code backend for **64-bit x86_64**. 
*   This allows the compiler's output to be executed and verified natively on modern Linux development machines.
*   `byte` and `word` values map cleanly to the 8-bit and 64-bit portions of x86 registers. 

### 7.4 Metadata and Debugging Observability
To assist users in verifying the compiler pipeline, the IR builder injects `SourceMarker` pseudo-instructions into the AST traversal. 
These markers propagate through the optimization phases down to the backends (X86_64, M6809, and CBE), emitting human-readable comments in the final assembly or C output (e.g., `; line 15: AssignStatement`). This makes correlating the machine code back to the original `.golf` script trivial.
