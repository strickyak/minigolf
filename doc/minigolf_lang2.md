# MiniGolf Language Reference

`github.com/strickyak/minigolf`

## 1. Introduction

MiniGolf (MINIature GO Language, Fun!) is a statically typed, compiled
systems programming language designed for low-level environments (such
as the 8-bit Motorola 6809) while offering modern ergonomics.

It is a strict subset of Go syntax, but the semantics are sometimes more like
the language C99.  Like C99, there is no assumption of a Heap or of Garbage Collection
built into the language, but they can be added by libraries, like malloc() and free() are
often available in C99 standard libraries.

## 1.02 Compiler Modes

The compiler is designed for "Whole Program Compilation" in which the source code
to all needed libraries is available to the compiler at once.

Working with precompiled libraries may be supported in the future.

### 1.02.1 Current M6809 Target Modes

The compiler is designed for multiple M6809 code generation scenarios:

*   RAW M6809 code with read-only `code` segment and read-write `data` segment.

### 1.02.2 Future M6809 Target Modes

*   OS9 Machine Language Module:  The read-only code segment must be relocatable
    (that is, uses Position Independant Code (PIC).   The data segment must be
    DP or some index-register -relative.

*   Same as above, but for Level II OS9 in which the data segment is always
    at $0000.

*   OS9 Device Driver and File Manager modes:  There is no single `main` entry,
    and there is no data segment for global variables.

## 1.05 Compiler Internals

The compiler is written in Go language, using both Gemini and Claude
for its general outline and for significant contributions of detailed
implementations.  Henry Strickland (github.com/strickyak ; email at
yak.net username strick ) is the Lead Engineer herding the robots.

The compiler has major phases:
*   Lexer: `Source Code -> Tokens`
*   Parser: `Tokens -> Abstract Syntax Tree (AST)`
*   Semantics: `AST -> AST`
*   Intermediate Representation (IR) Builder: `AST -> IR`
*   Optimizations: `IR -> IR`
*   Code Generation Back Ends (BE): `IR -> target platform`
*   Peephole Optimizations: `simplifies target assembly language output`

## 1.1. Back Ends

Multiple backends are already available:
*   **CBE:** The "C Backend" outputs C99 code, which can be compiled on 64-bit platforms.
*   **X86_64:** The "AMD64 Backend" outputs X86_64 assembly language, which can be compiled by modern gcc.
*   **M6809:** The "Motorola 6809 Backend" outputs 6809 assembly language, which can be compiled by the `lwasm` toolchain.

## 1.2. Front Ends

The only true front end (currently) is the MiniGolf lexer and parser. 

## 1.3. C to Minigolf Translator

However there is a separate program `cc_v5/cmd/cc_to_golf/` that 
takes preprocessed C code as input and produces a minigolf module as output.

## 2. Lexical Elements

MiniGolf's lexical structure mirrors Go.  Only ASCII characters are supported (not unicode).

*   **Comments:** Line comments `//` and block comments `/* ... */` are supported.
*   **Identifiers:** Begin with a letter or underscore, followed by letters, digits, or underscores.
*   **Keywords:** `package`, `import`, `func`, `var`, `const`, `type`, `struct`, `if`, `else`, `return`, `any`, `nil`, `for`.
*   **Literals:** Integer literals (decimal, octal, or hex), and ASCII string literals. String literals are assumed to be immutable and are allocated in the `code` section of the resulting binary.

In MiniGolf, as in Go, there are invisible semicolons at the end of statements.
They are inserted automatically for you by the parser.  However this means the
parser needs to know when you are finished with your statement, so use style like
shown here, which leave some part of the grammer unfinished when an end-of-statement
is not wanted yet:

GOOD:
```go
func g() {
    if ready > 0 {
      x := a +
              b
    } else {
        println("not ready")
    }
}
```

BAD:
```go
func g()
{
    if ready > 0
    {
      x := a
            + b
    }
    else {
        println
            ("not ready")
    }
}
```

Because MiniGolf syntax is a strict subset of Go syntax,
the Go language formatter `gofmt` can (and should) be used to format MiniGolf code.
`gofmt` settles all debates about correct MiniGolf style (:

## 3. Types

MiniGolf enforces strict typing. There are very few implicit type conversions. 

In this document, the word "object" is used loosely to mean any piece of data,
an instance of any type in the language.  MiniGolf does not have what an object-oriented
language defines as "objects", but it does have methods on struct types that
are a small, static subset of object-oriented language features.

### 3.1 Primitive Types
The primitive types:
*   `byte`: An 8-bit unsigned integer. Arithmetic overflows modulo $2^8$.
*   `word`: A pointer-sized unsigned integer (16-bit on the M6809 target). Arithmetic overflows modulo $2^{wordsize}$.
*   `uint`: an alias for `word`
*   `int`: A pointer-sized signed integer (16-bit on the M6809 target).
*   `bool`: An 8-bit integer with only 2 values: 0 and 1.
    * `true` is a predefined constant for 1
    * `false` is a predefined constant for 0.
*   `string`:  an alias for `prelude.slice[byte]`.  Whereas `string` and `[]byte` (slice of byte) are very different types in Go, they are the same in MiniGolf, implemented by `prelude.slice[byte]`.

### 3.2 Composite Types
*   **Arrays:** `[N]T` represents a contiguous, fixed-size array of `N` elements of type `T`. `N` must be a compile-time constant.
*   **Structs:** `struct { f1 T1; f2 T2; ... }` defines a contiguous memory layout of heterogeneous fields.

Arrays and Structs are *copied by value*, when assigned, when passed as parameters, and when returned from a function.

Actually in MiniGolf, *all types* are *copied by value*.  In Go, there are two types that are not copied by value: `map` and `chan` types, but MiniGolf does not have those types.  To be clear, for `slice[T]` (including `string`) and `any` and pointer types, the type behaves like a reference to other object(s), and it is this reference that is copied, not the referenced object(s).

### 3.3 Pointer Types
*   `*T` denotes a pointer to type `T`. Pointers hold the absolute memory address of a value. 
*   Pointer arithmetic is not supported directly; pointers must be cast to `word` if raw address manipulation is required.
    *  But see `pointer_add` and `pointer_sub` in the prelude, for helper functions.

* `func peek[T any](addr word) T` and `func poke[T any](addr word, x T)` are also provided in the Prelude.
    These can be used to manipulate raw memory.

### 3.5 Type `slice[T]`

A slice references some range of memory as contiguous elements of type T.
As with pointers in C99, you are responsible for keeping track of where that
memory came from and how it is to be freed.  There is no safety here.

* `slice[T]` (for any type T) is internally backed by a struct containing 3 fields:
    *   `Base word`: A pointer to the Base address, the address of the first element, with the index 0.
    *   `Len word`: the length of the slice, counted in elements of type T, not in bytes.
    *   `Cap word`: the capacity of the slice, counted in elements of type T, not in bytes.
        More elements of type T can be allocated from the storage behind the slice,
        up until `.Len == .Cap`.

Do use the `.Len` field for accessing the length of the slice.
Be more careful with the `.Base` and `.Cap` fields;
they are not needed unless you are crafting your own slice mechanisms.

*   Index syntax is supported for elements of slices: `x := mySlice[index]`
*   Indexes may be checked, if enabled in the compiler
*   Assignment is also supported by index: `mySlice[index] = x`
*   You may chop a slice into a subslice: `part := mySlice[ inclusive_start : exclusive_limit ]`

If your compiler mode has a heap mechanism like malloc/free or a garbage-collected heap allocation
mechanism, then you may use the Append method to construct slices:

```
func g() {
    ...

    var v slice[word]  // it starts out empty (the "zero value")
    for i := range 10 {
        v.Append(i)   // allocs and re-allocs/copies/frees the slice v as needed.
    }
    if v.Len != 10 {
        panic("OHNO")
    }

    ...
}
```

Append is defined in the prelude, if you need to change it.

If you know what you are doing, you are also welcome to construct your own slices:
```go
    v := slice[word]{ Base: myBase, Len: myLen, Cap: myCap }
```

### 3.6 Type `string`

*   `string` is an alias for the type `slice[byte]` which is a parameterized type defined in the prelude. 
*   Literal strings like `"Hello World"` are an object of type `slice[byte]` backed by read-only
    memory containing ASCII bytes.   These bytes are terminated by a 0 byte, as C strings are.
    The 0-termination byte does not count in the `.Len` of the string.
*   The type `*byte` is largely compatible with `string`, and might be sometimes used
    if we want a smaller reference object for the string, and we are willing to use 0 byte termination
    (or some other means) to find the Length of the string.
*   `string` supports lexical value comparisons `==`, `!=`, `<`, `>`, `<=`, `>=` that compare the
    values of the strings, not the address of the string.  This is a special exception, in which 
    `slice[byte]` behaves differently from any other struct type.
*   Initializers for `string` type are also a special exception:
    *   If the initialzer has field labels, the string object is initialized as its underlying struct type:
        `s := string { Base: 0x400, Len: the_len, Cap: the_cap }`
    *   If the initialzer has unlabled elements, the string object is initialized by bytes:
        `s := string { 'H', 'e', 'l', 'l', 'o' }`  ( Q: do we add a uncounted 0 byte? )
    *   If the initialzer is a string literal, it references that string literal's bytes:
        `s := "Hello World\n"`
    *   If the initialzer is another string, it copies the reference object, not the underlying bytes:
        `var t string ; s := t`

### 3.7 Type `any`

*   The builtin `any` type is the only type that would be considered an "interface" in Go.
*   An `any` can be created referencing an object of any data type.
*   Internally, an `any` is defined in the prelude to be backed by a struct containing
    two fields:  A pointer to the referenced data, and a `*byte` to a literal ASCII string contents
    naming the type of the data in a Human-friendly way, terminated by a 0 byte.

### 3.8 Allowed Conversions

*   **Pointers:** Pointer types can be cast to `word`, and `word` can be cast to pointer types.
*   **Bool:** Integer types can be used where a `bool` is required.  Nonzero integers convert to 1, and the integer 0 converts to 0.
*   **`*byte`:** Strings can be cast to `*byte` and `*byte` can be cast to `string`.
    In the later case, there will be a count of the bytes up to a 0 byte, to determine the `.Len` of the string.

### 3.9 Zero Values

*   All data types have a "zero value" which is a natural "zero" or "empty" or "nil" value for
    that type of object.
*   The "zero value" will actually have all bytes in the object set to 0 bytes.
*   All variables are initialized to their zero value, unless initialized to something else.

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
*   **Indexing and Slicing:** 
    *   `a[i]` indexes an array or a slice[T].
    *   `a[i] = ...` indexes an array or a slice[T] for assignment.
    *   `a[inclusive_start : exclusive_limit]` will chop a subslice from a slice.
*   **Type Conversions:** Explicit casts are required to change primitive types: `byte(x)` truncates a word to a byte; `word(x)` zero-extends a byte to a word.

## 6. Statements

Statements control execution flow.
*   **Assignments:** `x = y`.
    *   A simple `=` is used to assign existing variables.
    *   If one or more new variables are being created at this point, use `:=` instead.
        The new variable will have a type determined automatically by the type of the value assigned.
    *   New variables can also be created inside functions with a `var` statement: `var x int`
        They are initialized with a zero value unless initialized otherwise: `var x int = 888`
    *   Multi-value assignments are allowed:  `a, b, c := 1, 2, 3;  x, y = y, x`
    *   Multi-value unpacking is supported for struct fields (e.g., `a, b = myStruct`).
        *   The underlying reason is that internally, this is how functions return multiple values, via anonymous, synthetic structs

*   **Control Flow:**
    *   `if condition { ... } else { ... }`. The condition must evaluate to a comparison.
    *   `for { ... }`. Executes the block forever.
    *   `for condition { ... }`. Executes the block as long as the condition is true.
    *   `for i := range N { ... }` executes the block for i ranging from 0 to N-1
    *   `for k, v := range mySlice { ... }` executes the block for each element slice
    *   The special form `cond(p, y, n)` is like `( p ? y : n )` in C99.
        It looks like a function call, but all three of its arguments are not evaluated
        like in a function call.  Instead, the predicate p is evaluated first,
        and if p is true, the `y` is evaluated and becomes the result; otherwise, the `n`
        is evaluated and becomes the result.  The types of `y` and `n` must be the same
        (or use compatable constants).
*   **Increment / Decrement:** `x++` and `x--` are statements, not expressions.

## 7. Methods

MiniGolf supports receiver methods on struct types, enabling an object-oriented style without runtime dispatch overhead.

The method MUST have the receiver variable (like `recv` below) declared as a POINTER to the struct type.
We do not support receiver variables that are the struct type (like Go does).

```go
func (recv *Type) MethodName(args...) ReturnType {
    // ...
}
```

*   The receiver `recv` acts as an implicit first parameter. 
*   Method dispatch is entirely static. There are no interfaces or virtual method tables.
*   Either the object or a pointer to the object can be used at the call site:

```go
type Apple struct {}
func (recv *Apple) Mumble() { ,.. }
...
func g() {
    var apple Apple
    p := &apple
    apple.Mumble()  // address if apple is automatically passed to method
    p.Mumble()      // p is passed to the method
}
```

### 7.1 Destructors

If a struct type has a method `destructor()` (taking no arguments and returning no result)
then that method is guaranteed to be called on instances of the struct
that are local to a function or a method (that is, they are on the call stack)
when the function or method ends and the struct goes out of scope ( unless the
object has a zero value, in which case
the destructor may or may not be called;
it is implementation-depenedant).

This guarantee holds regardless of whether the flow of control hit an explicit
return function, "falls off the the bottom" of the function, or exits due
to a `panic()`.

(Notice we do not have "lexical scoping lifetimes" of variables declared in
nested blocks; those variables actually have a lifetime of the function or method,
even if they are not visible outside the nested block.)

The idea is to declare your destructable object with a zero value,
then only use appropriate methods to change it, which always leave your
object in a destructable state, and finally expect destructor() to be called
once (or optionally called, if you called no methods).

```go
func g() {
    var buf MyDestructableBuffer
    if changes != nil {
        buf.ApplyChanges(changes)
    }
    // destructor gets automatically called (or maybe not, if changes was nil).
}
```

## 8. Generics

MiniGolf supports a limited, compile-time monomorphization generic system for both types and functions. Type parameters are declared using the `[T any]` or `[A any, B any]` syntax.

### 8.1 Generic Types
Structs can be parameterized over one or more types to allow creating generalized data structures like linked lists or buffers.

```go
type Link[T any] struct {
    Value T
    Next  *Link[T]
}
```
*   The "noise word" `any` must occur as shown, for compatability with Go syntax.

*   When a generic type is used, it must be explicitly instantiated with concrete type arguments (e.g., `var myNode Link[byte]`).

*   Template expansion is done lexically in MiniGolf, not via an AST as in most languages.
    This is simpler to implement, but does not detect some errors as early.

### 8.2 Generic Functions
Functions can also declare type parameters.

```go
func First[T any](root *Link[T]) T {
    return root.Value
}
```
*   When a generic function is called (e.g., `First(&myNode)`), the compiler infers `T` from the argument types.
*   Upon inference, the compiler instantiates a unique, strongly-typed copy of the function for that specific set of type arguments.
*   If the type cannot be inferred from parameters, you may specify the type: `peek[byte](0x8000)`
*   Generic parameters abstract the exact memory layout of `T` while maintaining strict type safety during compilation.

## 9. Built-in Functions

To support basic debugging and bootstrapping without a standard library,
MiniGolf provides intrinsic functions:
*   `print(arg1, arg2, ...)`: Prints the provided arguments to standard output.
*   `println(arg1, arg2, ...)`: Prints the provided arguments followed by a newline character. 
*   `Sizeof[T]()`: Fill in an actual type for the letter T, and you get the size of the values.
    Unlike other things that look like function calls, this is a special form resulting in
    a const value, which can be used when a const value is needed (to define the length
    of an array or to define other const values).

The arguments to `print` or `println` can be
*   Integer literals or values of type `bool`, `byte`, `word`, or `int`.
*   `string` values, assuming the contents are printable ASCII.
*   `*byte` values, assuming the point to printable ASCII, 0-terminated strings.
*   `any`-typed references to supported typed values.

## 10.  Defer

Three forms of the `defer` statement are supported inside any function or method,
but only in the top level of its execution block (not in a nested block):

*   `defer f(a,b,c)`: defer a function call
*   `defer p.m(a,b,c)`: defer a method call
*   `defer func(){...}()`: defer execution of a block of code (that syntactially
    looks like a nested function, but is actually an ordinary block like the
    clauses of `if` or `for` are ordinary blocks.  MiniGolf does not have
    nested functions.)

When the flow of control leaves a function or method
(whether by explicit return, panic, or falling off the bottom of the function)
the `defer` statements are executed in backwards order.

The first two forms (function call and method call) have their arguments
(and method receiver) computed at the position in the function or method
where the `defer` statement occurs.   Those evaluated arguments are saved
and used when the defer'ed function or method is invoked.

The final form may be used for more complex cleanup.
It may use recover to catch a panic, and it may call panic
to start another panic (or effectively, to continue the
same panic).

If a panic was active when the defer'ed action is invoked,
and recover() does not occur without a new panic being started,
the panicking continues after the defer'ed action is invoked.

There is a strong commonality between `defer` invocatoin
and `destructor` invocation.  Both occur in the same chain
of deferred actions, in BACKWARDS ORDER of the order in which
they were introduced.

## 10. `panic` and `recover`

MiniGolf supports a panic/recover framework much like in Go.

A panic can occur in any function or method, either explicit or implicit.

An implicit panic might occur in situations like dereferencing a nil pointer,
an index exceeding the length of an array or slice, or division by 0.

An explicit panic is invoked by a builtin function `panic(p *byte) noreturn`.
(`noreturn` is a special type in MiniGolf that indicates a function will never return
by ordinary flow of control.)

Always call `panic` with a single parameter of type `*byte`, which must NOT be nil (zero),
and conventionally points to a short ASCII string describing the cause of the panic.
This string can be printed in error messages.

`func recover() *byte` looks to see if a panic is in progress,
and if so, returns the `*byte` parameter of the panic, and clears
the panic so we are no longer in a state of panicking.
If a panic was not in progress, `recover` returns nil.

`recover` is only useful in a defer'ed block with this 
conventional structure:

```go
func g() {
    ...

    defer func() {
        r := recover()
        if r != nil {
            // If we are here, there was a panic, but recover cleared it.
            if weCanHandle(r) {
                weDoHandle(r)
                // Great, we handled the situation,
                // so normal flow of control can continue,
                // exiting function g() with ordinary flow of control,
                // after any more destructors or deferred actions are done.
            } else {
                // We cannot handle the situation, so restart the same
                // (or a different) panic.
                panic(r)
            }
        } // or else there was no panic, so no recovery response is needed.
    }()

    ...
}
```

That is exactly as it is in Go, except that we constrain the panic value
and recover'ed value to be `*byte`, but in Go it can be any value.

## 11. `abort` and `exit`

If appropriate, the runtime library should provide two exits that are
more exceptional than `panic`:

*   `func abort(p *byte) notreturn` : Use your OS's process-aborting mechanism,
    which might produce debug artifacts like a core dump.

*   `func exit(status byte) noreturn` : Use your OS's process-exiting mechansm,
    which might use the status byte to signal success (a zero status) versus
    a non-successful (nonzero) status.

These exits violate the guarantee that defer'ed actions and destructors will
be called, because the process just goes away.

## 12. Package

Every module starts with a `package` statement,
for compatiblity with Go syntax.

It takes only one form:
```go
    package foo
```

The package name (like `foo`) must be present, but is always ignored.

The name of the package is actually one of two things:
*   If this is the main module whose file was named on the compiler command line, the module name is `main`.
*   If the package file named `bar.golf` was found by an `import "bar"` statement, it module name `bar`.

## 13. Import

Zero or more `import` statements may follow the `package` statement.

Import statements take only one form:
```go
    import "xyz"
```

The module "xyz" will be a source file named `xyz.golf` in the directory search path.
First the directory of the main module is searched.  Then any directories named with `-I`
flags on the command line are searched.

(Go's `flag` module is used for command-line flags, so Go's convention must be used:
Do not jam the directory name onto the -I flag like `-Imylib`.  You may use `=` or white
space, as in `-I=mylib` or `-I mylib`.)

Conventionally a standard library of modules in a directory named `golflib` is 
the last -I flag:  `-I $HOME/minigolf/golflib`

### 14.  Prelude

A module named `prelude` is always imported.  Its source will be in the search path,
named `prelude.golf`, like any other module.  However is very special, containing some
definitions essential to the language (like `type slice[T any]`).  Items defined in
the prelude enter the `builtin` namespace.  So you do not refer to things in the 
prelude with qualified names like `prelude.slice`; rather they have the simple name
like `slice`.

Some builtin names like `byte` and `word` and `nil` are so fundmental that they are
not defined in the prelude, but in the compiler.  They are in the builtin namespace, too.
