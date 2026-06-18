# Walkthrough: Register Tracking Bug Fix

I have successfully debugged and fixed the issue causing `test_func.golf` to produce incorrect math results (and which also caused other tests like `test_smap` to infinite loop).

## The Bug

The issue originated from how the M6809 backend handled register tracking for incoming parameters (Phase 7):
1. **Parameter Passing**: The backend correctly saves `firstWord` (passed in `X`) and `firstByte` (passed in `B`) to their stack slots at the very beginning of the function.
2. **Struct Copying**: For parameters larger than 2 bytes (like `s` in `eval(s string, w word)`), the backend emits `emitCopyYX` to copy the struct to the local stack frame. `emitCopyYX` clobbers `X`, `Y`, and `U` (as destination, source, and loop counter).
3. **The Conflict**: The compiler then explicitly marked `X` as containing `firstWord` (`w`) at the start of the first basic block. Since `X` had just been clobbered by the string copy, the compiler's register tracker thought the *garbage* in `X` was still the value of `w`.
4. **The Corruption**: When `flushRegisters()` was later called, it dutifully flushed this garbage value from `X` back into `w`'s stack slot (`32,s`), silently corrupting `w` with an absolute stack pointer address (`32594`).

## The Fix

I resolved this by tracking whether `X` or `B` gets clobbered during the initial parameter setup loop.

*   `xClobbered` and `bClobbered` boolean flags are introduced.
*   If `emitCopyYX` or `ldd`/`ldb` uses the registers during parameter copying, we flag them as clobbered.
*   At the start of the first basic block, we only inform the register tracker that `firstWord` is in `X` (and `firstByte` in `B`) **IF** they were not clobbered during parameter processing.
*   If they were clobbered, the compiler simply falls back to loading the parameters from their stack slots (where they were safely preserved at the very beginning of the function).

## Verification

I verified the fix by running:
*   `test_func.golf_m6809`: **Passed!**
*   `test_smap.golf_m6809`: **Passed!** (Previously hung in an infinite loop due to index register corruption)
*   `test_nil2.golf_m6809`: **Passed!**
*   `test_sort_strings.golf_m6809`: **Passed!**

This concludes the debugging session for `test_func` as requested. All mathematical operations and loops inside it are now evaluating perfectly in the M6809 backend!
