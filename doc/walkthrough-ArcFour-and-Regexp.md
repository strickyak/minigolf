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
    - `.` (match any character)
    - `|` (alternation)
    - `*` (Kleene star)
    - `?` (optional match)
    - `()` (grouping)
- `re.Compile(pat)` parses and constructs the Non-Deterministic Finite Automaton bytecode utilizing recursive descent.
- `re.Match(s)` and `re.MatchFull(s)` evaluates the NFA over the string iteratively, closing `epsilon` paths (resolving `JMP` and `SPLIT` nodes recursively via breadth-first search), thereby executing matches without risking recursion stack overflows on the hardware-limited `m6809`.

## Compiler IR Fixes
- Addressed an issue in `ir/backend_m6809.go` where binary shifts (`shl` and `shr`) assigned labels mapped directly to their shared `Instruction ID`. This caused "Multiply defined symbol" errors if the compiler encountered the same instruction structure sequentially (e.g. within tight nested VM epsilon closures). Migrated these loops to utilize the dynamically unique `b.nextLabel()` generator.
