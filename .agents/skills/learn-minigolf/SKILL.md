---
name: learn-minigolf
description: Learn the MiniGolf programming language by reading documentation and understanding the compiler architecture (lexer, parser, AST, IR, and three backends).
---

# Learn the MiniGolf Programming Language

## Documentation

Start by reading the language documentation:

```
doc/minigolf_lang.md
```

> **Note:** This document may be out of date. If you find omissions, ambiguities, or inconsistencies with the actual compiler behavior, please point them out.

## Compiler Architecture

The MiniGolf compiler follows a traditional pipeline: source → lexer → parser → AST → IR → backend code generation.

### Frontend

| Stage | File | Description |
|-------|------|-------------|
| Lexer | `lexer/lexer.go` | Tokenizes source code |
| Parser | `parser/parser.go` | Parses tokens into the Abstract Syntax Tree |
| AST | `ast/ast.go` | Abstract Syntax Tree data structures |
| IR | `ir/ir.go` | Intermediate Representation data structures |
| IR Builder | `builder/builder.go` | Transforms the AST into the IR |

### Backends

There are three supported backends:

| Backend | File | Target | Toolchain |
|---------|------|--------|-----------|
| **CBE** | `cbe/cbe.go` | C99 source code | GCC on Linux |
| **X86_64** | `x86_64/backend.go` | AMD64 Intel assembly | GCC on Linux |
| **M6809** | `m6809/backend.go` | Motorola 6809 assembly | `lwasm` |

### Project Goals

The true purpose of MiniGolf is to be an **optimizing compiler for the Motorola 6809**. We maintain all three backends as working targets for comparison and correctness validation. The M6809 backend is not yet well-optimized — improving its code generation is the primary goal of this project.

## Tests

Working tests are located in two directories:

| Directory | Language | Pattern |
|-----------|----------|---------|
| `c-tests/` | C99 input files | `c-tests/*.c` |
| `tests/` | MiniGolf input files | `tests/*.golf` |

Each test file has a corresponding `.want` file containing the golden (expected) output for that test. For example, `tests/hello.golf` would have `tests/hello.want`.

## Standard Library

The `golflib/` directory contains the standard library for the language. In particular, `golflib/prelude.golf` is **essential** — it is automatically included in every compilation.

## C99 Support

C99 source files are supported through a translation pipeline. Rather than compiling C99 directly, we:

1. Lex and parse the C99 source using a dedicated C frontend in `cc_v5/` (lexer, parser, and C-AST).
2. Translate the C-AST into MiniGolf source using `ctranslator/translator.go`.
3. Compile the resulting MiniGolf code with the standard MiniGolf compiler pipeline.

This means C99 support flows through: **C99 source → cc_v5 (lex/parse/C-AST) → ctranslator (emit MiniGolf) → MiniGolf compiler → backend**.
