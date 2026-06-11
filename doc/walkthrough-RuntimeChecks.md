# Panic Check Infrastructure Implementation

## Changes Made

### Command Line Flags
Added `-check-bounds` and `-check-nil` command-line flags to `main.go`, and their corresponding environment variables `CHECK_BOUNDS` and `CHECK_NIL`. These flags control the behavior of the `ir.Builder`.

### IR Builder Modifications
In `ir/builder.go`, we added:
1. `emitBoundsCheck`: Inserts bounds checks `idx >= limit` (or `idx > limit` for slice `Chop` limits). It emits a branch that invokes a panicking basic block with the message `INDEX OUT OF BOUNDS` if the check fails.
2. `emitNilCheck`: Inserts pointer null checks `ptr == 0`. It emits a branch to a panicking basic block with the message `NIL POINTER DEREFERENCE` if the check fails.

These runtime checks were surgically injected into:
- Array indexing and slice manipulations (`IndexExpression`). We differentiate standard indexing (`a[low]`) and slice chopping limits (`a[low:high]`).
- Pointer dereferences (`PrefixExpression` with `*`).
- Struct field accesses via pointer methods or properties (`SelectorExpression`).
- Indirect function calls.

### Validation Results
We constructed a suite of manual end-to-end tests:
- `tests/checkbounds2.golf`: Validates that `s[0:4]` successfully chops a slice of capacity 4, while `s[0:5]` cleanly invokes the `INDEX OUT OF BOUNDS` panic.
- `tests/checknil1.error.golf`: Validates that dereferencing a zero-initialized struct pointer crashes the program predictably with `NIL POINTER DEREFERENCE` rather than manifesting as arbitrary memory corruption later.

The changes integrate seamlessly with the existing Global Panic variables across backend runtimes (CBE, x86_64, M6809) resulting in uncaught panics terminating safely to stderr!
