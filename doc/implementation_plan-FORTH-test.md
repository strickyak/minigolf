# Forth Interpreter Implementation Plan

## Goal
Implement a small switch-threaded FORTH interpreter in `golflib/forth.golf` to successfully execute `tests/forth_count.golf`.

## User Review Required

> [!WARNING]
> The `prelude` library's `malloc` has a hardcoded panic if allocations exceed `TOO_BIG = 4000`. The test `forth_count.golf` specifically asks for `MEMORY_SIZE = 10000`. I will need to patch `prelude.go` to increase `TOO_BIG` and potentially `HEAP_SIZE` to support this contiguous allocation size safely! Is this acceptable?

## Proposed Architecture

This will be a **Switch-Threaded Forth VM**. Because MiniGo does not natively support function pointers, we will assign an integer Primitive ID to each built-in word (e.g., `PRIM_DUP = 1`). 

### VM State
```go
type Forth struct {
    Memory *byte
    Here   word    // Compilation pointer into Memory
    Latest word    // Pointer to latest Dictionary Header
    State  byte    // 0 = Interpret, 1 = Compile
    
    DataStack [64]word
    DPtr      byte
    
    RetStack  [64]word
    RPtr      byte
    
    CtrlStack [64]word // Control-flow stack for compiling DO/LOOP
    CPtr      byte
    
    IP        word // Instruction Pointer for inner interpreter
    
    Source    string // Input buffer
    SrcPos    word
}
```

### Dictionary Header Format
Created at `Here`, each word possesses a header linked list:
- `[2 bytes] Link`: Pointer to the previous Header (`f.Latest`)
- `[1 byte] LenFlags`: High-bit signifies `IMMEDIATE`. Lower 7 bits are the name length.
- `[N bytes] Name`: The string characters matching the name.
- `[2 bytes] Code Field (XT)`: Contains the Execution Token (XT), which is either a primitive ID, or `PRIM_DOCOL` for user-defined words.
- `[Variable] Parameter Field`: Follows the Code Field.

### Built-in Words needed for `forth_count.golf`:
- **Primitives**: `DUP`, `+`, `@`, `!`, `.`, `CR`, `I`.
- **Compiling Words (Immediate)**: 
    - `:` (Starts definition, compiles `PRIM_DOCOL`)
    - `;` (Compiles `XT_EXIT`, ends definition)
    - `DO` (Pushes `Here` to Control Stack, compiles `XT_DODO`)
    - `LOOP` (Pops Control Stack, compiles `XT_DOLOOP <target>`)
- **Other**: `VARIABLE` (Creates a dictionary entry containing `PRIM_DOVAR` and a single blank cell).

### The Inner Interpreter
The outer loop `Eval` reads tokens from `Source`. When compiling, it adds words to the dictionary.
When a compiled word (like `count`) is executed, the `Execute(xt)` method sets the `IP` (Instruction Pointer) to the start of the word's parameter field, and the inner interpreter runs:
```go
func (f *Forth) Inner() {
    for f.IP != 0 {
        xt := f.FetchWord(f.IP)
        f.IP += 2
        f.Execute(xt)
    }
}
```
`Execute(xt)` fetches the Code Field value `prim_id` from memory at `xt` and processes it via a large `switch` statement containing behaviors for all primitives and threaded structures.

## Verification
- Patch `prelude.go`'s `TOO_BIG` constant to `20000` to allow `malloc(10000)`.
- Execute `go test -v -count=1 . -run TestSystemAllGolfFiles/forth_count` to ensure standard compliance on `CBE`, `x86_64`, and `m6809` targets.
