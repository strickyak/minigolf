---
name: compile-and-run
description: Compile and run a `.c` or `.golf` source file with the MiniGolf compiler on all three target platforms (CBE, X86_64, M6809).
---

# Compile and Run with MiniGolf

Use `run4.sh` to compile and execute a `.c` or `.golf` source file with the MiniGolf compiler. This runs the program on all three supported platforms and compares the outputs.

## Usage

```bash
sh run4.sh SOURCE_FILENAME
```

Replace `SOURCE_FILENAME` with the path to a `.c` or `.golf` source file.

## What it does

1. Compiles the source file with the MiniGolf compiler.
2. Executes the compiled program on all three target platforms:
   - **CBE** (C Backend)
   - **X86_64** (x86-64 native)
   - **M6809** (Motorola 6809)
3. Shows the output of each platform's execution verbosely.
4. Compares the outputs across platforms using `md5sum` and `wc` (Unix word count) to verify consistency.

## Notes

- All three platforms should produce identical output for a correct program.
- If `md5sum` or `wc` results differ across platforms, investigate the platform-specific output for discrepancies.
