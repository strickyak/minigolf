# MiniGolf Language Reference

## 1. Introduction

MiniGolf (MINIature GO Language, Fun!) is a statically typed, compiled systems programming language designed for low-level environments (such as the 8-bit Motorola 6809) while offering modern ergonomics. It is a strict subset of Go syntax and semantics, intentionally constrained to eliminate runtime overhead, garbage collection, and complex typing rules. 

This document serves as a rigorous reference for programmers familiar with C, C++, or Go.

## 2. Lexical Elements

MiniGolf's lexical structure mirrors Go.
*   **Comments:** Line comments `//` and block comments `/* ... */` are supported.
*   **Identifiers:** Begin with a letter or underscore, followed by letters, digits, or underscores.
*   **Keywords:** `package`, `import`, `func`, `var`, `const`, `type`, `struct`, `if`, `else`, `while`, `return`, `any`.
    *   *Note:* MiniGolf uses `while` instead of Go's `for` for looping.
*   **Literals:** Integer literals (decimal), and ASCII string literals. Strings are strictly immutable and can only be passed directly to built-in print functions.

## 3. Types

MiniGolf enforces strict typing. There are no implicit type conversions. 

### 3.1 Primitive Types
There are exactly two primitive numeric types. Both are unsigned:
*   `byte`: An 8-bit unsigned integer. Arithmetic overflows modulo $2^8$.
*   `word`: A pointer-sized unsigned integer (16-bit on the M6809 target). Arithmetic overflows modulo $2^{16}$.

### 3.2 Composite Types
*   **Arrays:** `[N]T` represents a contiguous, fixed-size array of `N` elements of type `T`. `N` must be a compile-time constant.
*   **Structs:** `struct { f1 T1; f2 T2; ... }` defines a contiguous memory layout of heterogeneous fields.

### 3.3 Pointer Types
*   `*T` denotes a pointer to type `T`. Pointers hold the absolute memory address of a value. 
*   Pointer arithmetic is not supported directly; pointers must be cast to `word` if raw address manipulation is required.

## 4. Declarations

A MiniGolf program consists of a single flat module containing one or more files. Every file must begin with a `package` declaration (compilation targets require `package main`).

Top-level declarations define the file's structure:
*   **Types:** `type Name UnderlyingType` (e.g., `type Apple struct { worms byte }`).
*   **Constants:** `const Name = Value`. Constants are evaluated at compile time.
*   **Variables:** `var Name Type` allocates global memory.
*   **Functions:** `func Name(param Type) ReturnType { ... }`.

## 5. Expressions

Expressions compute values. Operands in binary expressions must be of the exact same type.

*   **Arithmetic:** `+`, `-`, `*`, `/`, `%`
*   **Bitwise:** `&`, `|`, `^`, `<<`, `>>`
*   **Logical / Comparison:** `==`, `!=`, `<`, `<=`, `>`, `>=`. Comparisons evaluate conceptually to a boolean, represented internally as a `byte` (0 for false, 1 for true).
*   **Addressing and Dereferencing:** 
    *   `&x` yields a pointer (`*T`) to the operand `x`. The operand must be addressable (an L-value like a variable or struct field).
    *   `*p` yields the value of type `T` pointed to by `p`.
*   **Type Conversions:** Explicit casts are required to change primitive types: `byte(x)` truncates a word to a byte; `word(x)` zero-extends a byte to a word.

## 6. Statements

Statements control execution flow.
*   **Assignments:** `x = y`. Multi-value unpacking is supported for struct fields (e.g., `a, b = myStruct`).
*   **Control Flow:**
    *   `if condition { ... } else { ... }`. The condition must evaluate to a comparison.
    *   `while condition { ... }`. Executes the block as long as the condition is true.
*   **Increment / Decrement:** `x++` and `x--` are statements, not expressions.

## 7. Methods

MiniGolf supports receiver methods on user-defined pointer types, enabling an object-oriented style without runtime dispatch overhead.

```go
func (recv *Type) MethodName(args...) ReturnType {
    // ...
}
```
*   The receiver `recv` acts as an implicit first parameter. 
*   Method dispatch is entirely static. There are no interfaces or virtual method tables.

## 8. Generics

MiniGolf supports a limited, compile-time monomorphization generic system for functions.

```go
func FunctionName[T any](ptr *T) {
    // ...
}
```
*   Functions can declare type parameters using the `[T any]` syntax.
*   When a generic function is called (e.g., `FunctionName(&myVar)`), the compiler infers `T` from the argument types and instantiates a unique, strongly-typed copy of the function.
*   Currently, generic parameters are primarily intended for pointer manipulation and memory operations where the exact layout of `T` is abstracted.

## 9. Built-in Functions

To support basic debugging and bootstrapping without a standard library, MiniGolf provides intrinsic functions:
*   `print(arg1, arg2, ...)`: Prints the provided arguments to standard output.
*   `println(arg1, arg2, ...)`: Prints the provided arguments followed by a newline character. 

*Note: Built-in print functions are the only context where string literals may be evaluated.*
