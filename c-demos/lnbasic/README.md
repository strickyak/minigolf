# Line Number BASIC (`lnbasic`)

An ANSI C (C89) Line Number BASIC interpreter written for the MiniGolf compiler and freestanding Motorola 6809 systems, as well as native Unix/Linux.

## Features

- **Strict ANSI C (C89)**:
  - Compiles cleanly with `gcc -std=c89 -pedantic -Wall -Werror`.
  - Zero library or header dependencies on bare-metal targets: no `stdio.h` or standard C library needed.
  - `#if unix` includes `<stdio.h>` and standard `putchar()`/`getchar()` on Linux/Unix.
  - Freestanding backend directly writes characters to memory-mapped port `$FF00` and reads non-zero keystroke bytes from port `$FF01`.
- **Supported Statements**:
  - `LET <var> = <expr>` (also supports implicit assignment `<var> = <expr>`)
  - `PRINT ...` (string literals `"..."`, expressions, `;` compact spacing, `,` tab stops)
  - `FOR <var> = <start> TO <end> [STEP <step>]` ... `NEXT [<var>]`
  - `REM ...` (comments)
  - `IF <cond> THEN <stmt|line_number>`
  - `GOTO <line_number>`
- **Supported Commands**:
  - `NEW`: Clears stored program and resets variables.
  - `LIST`: Displays program lines in ascending line number order.
  - `RUN`: Executes the stored program from the lowest line number.
  - `BYE`: Exits the BASIC interpreter.
- **Line Editing**:
  - Entering a line starting with a positive number inserts or replaces that line in sorted order.
  - Entering a line number with no statement removes that line.

## Building and Running

### With GCC (Linux)

```bash
make
./lnbasic
```

Or pipe a script directly:

```bash
(cat demo.bas; echo "RUN"; echo "BYE") | ./lnbasic
```

### With MiniGolf for Motorola 6809

Using `make` (assembles `lnbasic.decb` using `minigolf` and `lwasm`, then runs on `hatvan-vm`):

```bash
make test
```

Manual steps:

1. Compile `lnbasic.c` to M6809 assembly:
   ```bash
   minigolf -m=m6809 -o lnbasic_core.asm lnbasic.c
   ```
2. Combine with `cstart.asm` and assemble:
   ```bash
   cat cstart.asm lnbasic_core.asm > lnbasic.asm
   echo "    end cstart" >> lnbasic.asm
   lwasm -9 -b -f decb -o lnbasic.decb lnbasic.asm
   ```
3. Run under the Hatvan OS VM:
   ```bash
   hatvan-vm --input='10 PRINT "HELLO"\nRUN\nBYE\n' lnbasic.decb
   ```
