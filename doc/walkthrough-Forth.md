# Walkthrough: ARCFOUR (RC4) Implementation

I've successfully created the `arcfour` library module containing the `ArcFour` structure and core functionality matching the standard RC4 specification algorithm, along with tests verifying against Wikipedia's official test vectors!

## Technical Implementation

### `Init(key *byte)` 
The initialization performs standard RC4 Key-Scheduling Algorithm (KSA). To determine the key length of the string without relying on higher-level generic functions, I iterate over the `*byte` pointer until hitting the `0` string terminator using pointer arithmetic (`word` casting). The identity array `S[0...255]` is populated and shuffled based on the N-length byte key.

### `Next() byte`
The core Pseudo-Random Generation Algorithm (PRGA) modifies the internal pointers `i` and `j`, yields the array swap, and correctly computes the resultant pseudo-random byte.

### `Crypt(a string)`
This operates over `string` instances in-place. Because MiniGo implements strings using strict byte pointers and lengths (underneath), I extracted the underlying `*byte` through a fast private pointer mapping helper function (`strptr`) to mutate the structure directly.

### Verification and Platform Compatibility
To test the output:
- I correctly set up the expected encryptions for `Key/Plaintext`, `Wiki/pedia`, and `Secret/Attack at dawn`. 
- **CRITICAL**: Modern backends (`x86_64`) often map literal strings into `.rodata` (read-only memory segments), causing segmentation faults when attempting in-place encryption! To solve this and keep the implementation clean, I dynamically wrapped the plaintext inputs inside our tests with `strdup`, safely dumping the string onto the heap allowing `Crypt` to perform its fast in-place modifications without memory violation!
- Test assertions pass effortlessly on **all** targets (`x86_64`, `CBE`, and `m6809`).

# Walkthrough: Regexp Virtual Machine

I implemented a `Regexp` object-oriented library located at `golflib/regexp.golf`. Because MiniGo does not offer a garbage collector or dynamic memory (`make`), the library evaluates regular expressions by compiling them into a bytecode format mimicking Thompson's NFA algorithm within a static `[256]word` buffer.

## Technical Implementation
- **Features supported**: 
    - The Thompson NFA successfully simulates alternative (`|`), Kleene Star (`*`), optional (`?`), and wildcard (`.`) matching.
- It passed complex backtracking edge-cases natively across multiple backend compilers.

---

## 4. Forth Virtual Machine (`golflib/forth.golf`)

Implemented a switch-threaded Forth interpreter designed natively in `minigolf` to accommodate tight constraints across multiple architectures (`cbe`, `x86_64`, `m6809`).

### Architecture Adjustments
- **Pointer Alignment**: Dictionary headers were shifted from packed `byte` representations to strict `word`-aligned pointers and fields to alleviate segmentation faults on the `x86_64` backend.
- **Global Memory Isolation**: Advanced Forth VM state components (like Memory arrays) were instantiated in the global scope to bypass severe stack constraints on the `m6809` target.
- **Core Primitives Supported**: `dup`, `+`, `-`, `*`, `/`, `mod`, `@`, `!`, `.`, `cr`, `i`, `:`, `;`, `variable`, `do`, `loop`.

### The Memory Wrap-Around Mystery (`m6809`)
During multi-backend testing, the `m6809` emulator appeared to silently hang during Forth initialization. It was discovered that the issue was not related to signed division or negative numbers as initially suspected.
Instead, the `m6809` processor has a maximum 64KB (16-bit) address space. `HEAP_SIZE` was previously computed inside the compiler's `prelude.go` as `20000 + 32 * 512 = 36384` bytes. Allocating a global BSS array of 36KB on a 64KB machine consumed over half of the entire address space. 

When combined with the emulator's existing ROM, system variables, and stack, this oversized allocation caused the BSS segment to overflow `0xFFFF` and wrap around memory back to the direct page (`0x0000`). This silently overwrote the stack or Gomar hardware registers, triggering a hardware fault or infinite loop before memory allocation `malloc_init` could even execute.

By decreasing the `HEAP_SIZE` multiplier from `512` to `256`, the heap was reduced to a comfortable `28192` bytes (28KB), which easily fits into the memory map without wrap-around. The `do...loop` Forth test now executes flawlessly natively on `m6809`!

## Compiler IR Fixes
- Addressed an issue in `ir/backend_m6809.go` where binary shifts (`shl` and `shr`) assigned labels mapped directly to their shared `Instruction ID`. This caused "Multiply defined symbol" errors if the compiler encountered the same instruction structure sequentially (e.g. within tight nested VM epsilon closures). Migrated these loops to utilize the dynamically unique `b.nextLabel()` generator.
