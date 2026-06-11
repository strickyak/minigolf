# Writing a New Backend for MiniGolf

This guide explains the architecture of the MiniGolf Intermediate Representation (IR) and provides a reference for writing a new backend (e.g., for target architectures like the Motorola 68000, x86_64, or a C-Backend).

## The Intermediate Representation (IR)

The MiniGolf compiler converts the Abstract Syntax Tree (AST) into a flat, typed, Static Single Assignment (SSA) Intermediate Representation. 
The IR is structured as a `Program` containing `Globals`, type definitions, and `Functions`.
Each `Function` contains a set of `BasicBlock`s, which consist of sequential `Instruction`s and end with a `Terminator` instruction (like a branch or return).

A new backend's primary job is to implement a `Generate(program *ir.Program) string` method that:
1. Emits global variables, string constants, and static data.
2. Iterates over functions.
3. Allocates stack slots for all SSA values (and local variables) used in each function.
4. Translates each IR instruction into one or more target machine instructions.

### Basic Concepts
- **SSA Values**: Every instruction producing a value assigns it to a virtual register, represented by its ID (`v1`, `v2`, etc.). Because it's SSA form, these are written to exactly once. Most backends will map these virtual registers to memory locations on the stack ("spill slots") or physical registers.
- **Types**: The IR is typed. Primitive types include `byte`, `word`, `int`, `const_integer`, and `void`. There are also composite types like structs, arrays, and pointers. The backend uses the type to know the size of a load, store, or arithmetic operation.
- **Phi Nodes**: Because MiniGolf uses SSA, control flow merges use `phi` nodes to select a value based on the predecessor block. Most backends implement `phi` nodes by resolving them at the end of the predecessor block, copying the value from the predecessor into the target `phi` node's allocated stack slot/register.

## Translating Instructions

When visiting an instruction in a basic block, the backend matches its type and emits assembly. Below is an exhaustive list of the IR instructions defined in `ir/ir.go` and how they are typically implemented.

### Constant Instructions
These instructions introduce constant values into an SSA register.
* **`ConstByte` (`const_byte`)**: Load an 8-bit immediate value into the destination slot.
* **`ConstWord` (`const_word`)**: Load a 16-bit, 32-bit, or 64-bit immediate (depending on the target's word size) into the destination slot.
* **`Sizeof` (`sizeof`)**: Compute the size of the target type in bytes and store the integer constant.
* **`ConstStruct` (`const_struct`)**: Initialize a struct value, typically by pushing or copying its individual fields into the destination slot.
* **`ConstArray` (`const_array`)**: Initialize an array value with a list of element values.

### Memory Operations
* **`Load` (`load`)**: Read the value from a specific `Global` variable into an SSA register.
* **`Store` (`store`)**: Write an SSA register's value directly to a `Global` variable.

### Pointer and Address Operations
Pointers represent memory addresses. Operations with `_ptr` suffixes perform indirect memory accesses using addresses calculated or loaded into SSA values.
* **`AddressOfGlobal` (`addrof`)**: Get the memory address of a `Global` variable.
* **`AddressOfLocal` (`addrof_local`)**: Get the memory address of a local variable or a spilled SSA value on the stack.
* **`AddressOfFunc` (`addrof_func`)**: Get the function pointer/address for an `ir.Function`.
* **`AddressOfField` (`addrof_field`)**: Given a pointer to a struct, calculate the address of a specific field (adds the field's offset to the base pointer).
* **`AddressOfElement` (`addrof_element`)**: Given a pointer to an array and an index, calculate the address of the element (adds `index * sizeof(element)` to the base pointer).
* **`ExtractFieldPtr` (`extract_field_ptr`)**: Given a pointer to a struct, load the value of a specific field into an SSA register.
* **`InsertFieldPtr` (`insert_field_ptr`)**: Given a pointer to a struct, store a value into a specific field.
* **`LoadPtr` (`load_ptr`)**: Dereference a pointer SSA value and load the target memory into a new SSA register.
* **`StorePtr` (`store_ptr`)**: Store an SSA value into the memory address held by a pointer SSA value.

### Value-Based Struct and Array Operations
These operate on entire structs/arrays held in registers/slots, not via pointers.
* **`ExtractElement` (`extract`)**: Extract an element from an array value at a specific index.
* **`InsertElement` (`insert`)**: Create a new array value by replacing the element at a specific index.
* **`ExtractField` (`extract_field`)**: Extract the value of a specific field from a struct value.
* **`InsertField` (`insert_field`)**: Create a new struct value by replacing a specific field.

### Arithmetic & Logic Operations
* **`BinaryOp`**: Performs an operation on a `Left` and `Right` operand. Target size depends on the operands' types.
  * Opcodes: `add`, `sub`, `mul`, `div`, `mod`, `and`, `or`, `xor`, `shl`, `shr`.
* **`Compare`**: Compares `Left` and `Right` operands and returns a boolean `byte` (1 for true, 0 for false).
  * Opcodes: `eq`, `neq`, `lt`, `lte`, `gt`, `gte`.
* **`UnaryOp`**: Performs an operation on a single operand.
  * Opcodes: `not` (bitwise or logical NOT depending on context), `neg` (two's complement negation).

### Type Conversions
* **`Cast`**:
  * `zero_ext`: Zero-extend a smaller integer type (e.g., `byte`) to a larger integer type (e.g., `word`).
  * `trunc`: Truncate a larger integer type to a smaller integer type.

### Function Calls
* **`Call` (`call`)**: Call a known MiniGolf `Function` using its label. Pass `Args` (usually pushed to the stack or placed in registers according to the calling convention) and retrieve the return value.
* **`IndirectCall` (`indirect_call`)**: Call a function pointer held in an SSA register.
* **`BuiltinCall` (`builtin_<name>`)**: Call a compiler builtin (e.g., `print`, `println`). The backend must manually implement the behavior of the builtin, often by calling standard library functions like `printf`.

### Terminators
Terminators end a basic block and manage control flow.
* **`Jump` (`jmp`)**: Unconditional branch to the target `BasicBlock`.
* **`Branch` (`br`)**: Conditional branch based on a `Condition` SSA value. If non-zero, jump to `TrueBlock`; otherwise, jump to `FalseBlock`.
* **`Return` (`ret`)**: Return from the current function, optionally placing the `Val` (if not void) in the return register or stack slot.
* **`SetJmp` (`setjmp`)**: Intrinsic to save execution context for `panic` handling.
* **`LongJmp` (`longjmp`)**: Intrinsic to restore execution context, used by `panic`.

### Miscellaneous
* **`ZeroInit` (`zeroinit`)**: Produce a value of the target type where all bytes are zero (e.g., `{0}`, `0`, `NULL`).
* **`SourceMarker` (`source_marker`)**: A no-op instruction used for debugging. Backends can emit its `Comment` field as a comment in the generated assembly to map assembly lines back to original MiniGolf source code.

## Backend Implementation Strategy
A typical backend like `x86_64` or `m6809` follows these phases:

1. **Stack Allocation**: Iterate over all `Function`s, their `BasicBlock`s, and `Instruction`s. Assign a stack offset/slot for every local variable and SSA virtual register (`v1`, `v2`, etc.). Ensure parameters are mapped to arguments provided by the caller.
2. **Global Emitting**: Emit data sections (`.data` or `.bss`) for `ir.Global` variables, taking into account initialization values.
3. **Instruction Translation**: For each block, loop over the instructions. Use a `switch` on the instruction type (e.g., `*ir.BinaryOp`, `*ir.LoadPtr`). Emit the machine code to load the operands from their stack slots, perform the operation, and store the result in the instruction's own stack slot.
4. **Phi Elimination**: Instead of generating code for `phi` instructions when they are encountered, resolve them at the end of predecessor blocks. For every outgoing edge to a block with a `phi`, emit `mov` or `store` instructions to copy the predecessor's value into the `phi` node's stack slot.
5. **Calling Convention**: Decide how arguments are passed (e.g., pushed to the stack, passed in registers) and how return values are given back. Implement prologues (`push rbp; mov rbp, rsp`) and epilogues correctly.
