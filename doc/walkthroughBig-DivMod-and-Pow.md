# Walkthrough: DivMod and Pow implementation

I have successfully implemented the `DivMod` and `Pow` functions for big decimal math in `golflib/big.golf`.

## Technical Details

### `DivMod` via Base-10 Long Division
I implemented a highly efficient base-10 long division algorithm specifically tailored to `big.Dec`'s array structure:
1. **`CmpShifted` and `SubShifted`**: Instead of instantiating an explicitly shifted divisor taking up massive amounts of memory, I added in-place offset-comparison and offset-subtraction routines that directly operate over `a` and `b`.
2. **Repeated Subtraction**: For every "shift" magnitude (starting from `r.Size - b.Size` down to `0`), it efficiently compares the remainder against the shifted divisor and uses repeated subtraction to cleanly construct the quotient digits without any explicit `div` instructions, bypassing backend discrepancies entirely.

### Pointer Arguments
As discussed and approved, I went with `big.DivMod(&q, &r, &a, &b)`. Because `Dec` relies on a full 255-byte capacity array structure, returning multiple values would needlessly copy `512 bytes` onto the stack for every division operation! The pointer-based API ensures maximum efficiency and memory stability in MiniGo.

### `Pow` via Modular Exponentiation
Using the new `DivMod` routine, I implemented standard Right-to-Left binary modular exponentiation. It raises `a` to the power of `b` under a specific modulus `m` by continually dividing the exponent by 2 via `Div2`, saving exponential operation steps.

## Verification

The new logic was exhaustively tested across the new `tests/test_big_mul.golf` file. Tests confirmed correct extraction of Quotient (`21`) and Remainder (`3`) from `255 / 12`, as well as producing the correct answer for `2^10 mod 1000 = 24` and `2^10 mod 10000 = 1024`.

All test assertions successfully passed for every target: `x86_64`, `CBE`, and `m6809`!
