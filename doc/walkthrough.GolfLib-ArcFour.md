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
